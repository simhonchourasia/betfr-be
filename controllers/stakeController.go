package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/simhonchourasia/betfr-be/config"
	"github.com/simhonchourasia/betfr-be/database"
	"github.com/simhonchourasia/betfr-be/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var stakeCollection *mongo.Collection = database.OpenCollection(database.Client, config.GlobalConfig.StakeCollection)

// Pass in
var CreateStakeFunc gin.HandlerFunc = func(c *gin.Context) {
	var ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var stakeReq models.StakeRequest

	if err := c.BindJSON(&stakeReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if validationErr := validate.Struct(stakeReq); validationErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
		return
	}

	stake := models.Stake{
		ID:             primitive.NewObjectID(),
		Underlying:     stakeReq.Underlying,
		SharesStaked:   stakeReq.NumShares,
		SharesFilled:   0,
		BackingCreator: stakeReq.BackingCreator,
		Comment:        stakeReq.Comment,
		CreateDate:     primitive.NewDateTimeFromTime(time.Now()),
	}

	betResult := betCollection.FindOne(ctx, bson.M{"_id": stakeReq.Underlying})
	var bet models.Bet
	if betResult.Err() != nil {
		fmt.Println(betResult.Err().Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Underlying ID %s not found", stakeReq.Underlying.String())})
		return
	}
	if err := betResult.Decode(&bet); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if bet.CreatorStakedUnfilled != 0 && bet.ReceiverStakedUnfilled != 0 {
		panic(fmt.Errorf("bet %s has nonzero unfilled amounts for both sides. this should not happen", bet.ID.String()))
	}
	if bet.CreatorStakedUnfilled < 0 || bet.ReceiverStakedUnfilled < 0 {
		panic(fmt.Errorf("bet %s has negative filled amount for a side. this should not happen", bet.ID.String()))
	}

	if stakeReq.BackingCreator {
		bet.CreatorStaked += stake.SharesStaked
		if stake.SharesStaked <= bet.ReceiverStakedUnfilled {
			// Case 1: We can fully fill this stake; it will not go into the bet's queue
			stake.SharesFilled = stake.SharesStaked
			bet.ReceiverStakedUnfilled -= stake.SharesFilled
		} else {
			// Case 2: We can partially fill this stake (or not at all)
			// It will sit in the bet's queue until it can be further filled
			// And it may help fill other stakes on the other side
			bet.CreatorStakes = append(bet.CreatorStakes, stake.ID)
			stake.SharesFilled = bet.ReceiverStakedUnfilled
			bet.ReceiverStakedUnfilled = 0
			bet.CreatorStakedUnfilled += stake.SharesStaked - stake.SharesFilled
			// Then go through the receiver stake queue and fill up as many as possible
			if err := HandleStakes(ctx, &bet); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	} else {
		bet.ReceiverStaked += stake.SharesStaked
		// Stake is backing up the Receiver
		// Symmetric logic to when creator is backed up
		if stake.SharesStaked <= bet.CreatorStakedUnfilled {
			// Case 1: We can fully fill this stake; it will not go into the bet's queue
			stake.SharesFilled = stake.SharesStaked
			bet.CreatorStakedUnfilled -= stake.SharesFilled
		} else {
			// Case 2: We can partially fill this stake (or not at all)
			bet.ReceiverStakes = append(bet.CreatorStakes, stake.ID)
			stake.SharesFilled = bet.CreatorStakedUnfilled
			bet.CreatorStakedUnfilled = 0
			bet.ReceiverStaked += stake.SharesStaked - stake.SharesFilled
			// Then go through the receiver stake queue and fill up as many as possible
			if err := HandleStakes(ctx, &bet); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	// Update bet and insert stake
	opts := options.Replace().SetUpsert(true)
	filter := bson.M{"_id": bet.ID}

	replaceRes, err := betCollection.ReplaceOne(
		ctx,
		filter,
		bet,
		opts,
	)

	if err != nil || replaceRes.MatchedCount == 0 {
		log.Printf("Could not find/update bet for stake\n")
		if err == nil {
			err = fmt.Errorf("bet did not previously exist")
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	res, err := stakeCollection.InsertOne(ctx, stake)
	if err != nil {
		log.Printf("Could not create stake\n")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, res)
}

func updateStakeFilledHelper(ctx context.Context, stake models.Stake) error {
	res, err := stakeCollection.UpdateOne(
		ctx,
		bson.M{"_id": stake.ID},
		bson.D{{Key: "$set", Value: bson.D{{Key: "sharesfilled", Value: stake.SharesFilled}}}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("could not find stake with ID %s to update", stake.ID.String())
	}
	return nil
}

// Handles all stakes for a bet
// To be called when a stake is created
func HandleStakes(ctx context.Context, bet *models.Bet) error {
	if bet.CreatorStakedUnfilled != 0 && bet.ReceiverStakedUnfilled != 0 {
		panic(fmt.Errorf("bet %s has nonzero unfilled amounts for both sides. this should not happen", bet.ID.String()))
	}
	if bet.CreatorStakedUnfilled < 0 || bet.ReceiverStakedUnfilled < 0 {
		panic(fmt.Errorf("bet %s has negative filled amount for a side. this should not happen", bet.ID.String()))
	}

	filledIdx := 0 // At the end, we will have queue[filledIdx:] as our new queue (as we might pop things off the queue)
	queueName := ""

	if bet.CreatorStakedUnfilled > 0 {
		// Case for when we are trying to fill up receiver stakes with a new surplus of creator stakes
		queueName = "receiverstakes"

		for _, stakeID := range bet.ReceiverStakes {
			if bet.CreatorStakedUnfilled > 0 {
				// Get next receiver stake in the queue and operate
				stakeRes := userCollection.FindOne(ctx, bson.M{"_id": stakeID})
				if stakeRes.Err() != nil {
					return stakeRes.Err()
				}
				var currStake models.Stake
				if err := stakeRes.Decode(&currStake); err != nil {
					return err
				}

				// Try to fill it up as much as possible
				remainder := currStake.SharesStaked - currStake.SharesFilled
				if bet.CreatorStakedUnfilled < remainder {
					// Case 1: We can only fill the queued stake partially, so we do that and update the stake and then end
					// Note that we do not remove anything from the queue in this case
					currStake.SharesFilled += bet.CreatorStakedUnfilled
					bet.CreatorStakedUnfilled = 0
					updateStakeFilledHelper(ctx, currStake)
					break
				} else {
					// Case 2: We can fill up the queued stake entirely, so we do that and pop it from the queue and move on
					// Note that we will remove things from the queue in this case
					currStake.SharesFilled += remainder
					bet.CreatorStakedUnfilled -= remainder
					filledIdx += 1
					updateStakeFilledHelper(ctx, currStake)
				}
			}
		}
	} else {
		// Case for when we are trying to fill up creator stakes with a new surplus of receiver stakes
		// Symmetric logic to above
		queueName = "creatorstakes"

		for _, stakeID := range bet.CreatorStakes {
			if bet.ReceiverStakedUnfilled > 0 {
				// Get next receiver stake in the queue and operate
				stakeRes := userCollection.FindOne(ctx, bson.M{"_id": stakeID})
				if stakeRes.Err() != nil {
					return stakeRes.Err()
				}
				var currStake models.Stake
				if err := stakeRes.Decode(&currStake); err != nil {
					return err
				}

				// Try to fill it up as much as possible
				remainder := currStake.SharesStaked - currStake.SharesFilled
				if bet.ReceiverStakedUnfilled < remainder {
					// Case 1: We can only fill the queued stake partially, so we do that and update the stake and then end
					// Note that we do not remove anything from the queue in this case
					currStake.SharesFilled += bet.ReceiverStakedUnfilled
					bet.ReceiverStakedUnfilled = 0
					updateStakeFilledHelper(ctx, currStake)
					break
				} else {
					// Case 2: We can fill up the queued stake entirely, so we do that and pop it from the queue and move on
					// Note that we will remove things from the queue in this case
					currStake.SharesFilled += remainder
					bet.ReceiverStakedUnfilled -= remainder
					filledIdx += 1
					updateStakeFilledHelper(ctx, currStake)
				}
			}
		}
	}

	newQueue := bet.ReceiverStakes[filledIdx:]
	res, err := betCollection.UpdateOne(
		ctx,
		bson.M{"_id": bet.ID},
		bson.D{{Key: "$set", Value: bson.D{{Key: queueName, Value: newQueue}}}}, // hopefully this is the right way to do it
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("could not find bet with ID %s to update for stake", bet.ID.String())
	}

	return nil
}

// Pays out stake owners
// To be called when a bet is resolved
// Instead of trying to match up stakes on both sides, the stake winners will be owed by the original bet's loser
// And the original bet's loser will be owed by the stake losers
func PayoutStakes(ctx context.Context, bet *models.Bet) error {
	creatorRes := userCollection.FindOne(ctx, bson.M{"username": bet.CreatorName})
	if creatorRes.Err() != nil {
		return fmt.Errorf("bet creator %s not found", bet.CreatorName)
	}
	var creator models.User
	if err := creatorRes.Decode(&creator); err != nil {
		return err
	}
	receiverRes := userCollection.FindOne(ctx, bson.M{"username": bet.ReceiverName})
	if receiverRes.Err() != nil {
		return fmt.Errorf("bet creator %s not found", bet.ReceiverName)
	}
	var receiver models.User
	if err := receiverRes.Decode(&receiver); err != nil {
		return err
	}

	for _, stakeID := range bet.CreatorStakes {
		stakeRes := stakeCollection.FindOne(ctx, bson.M{"_id": stakeID})
		if stakeRes.Err() != nil {
			return fmt.Errorf("stake id %s not found", stakeID.String())
		}
		var stake models.Stake
		if err := stakeRes.Decode(&stake); err != nil {
			return err
		}

		creatorStakerRes := userCollection.FindOne(ctx, bson.M{"username": stake.OwnerName})
		if creatorRes.Err() != nil {
			return fmt.Errorf("stake winner %s not found", stake.OwnerName)
		}
		var creatorStaker models.User
		if err := creatorStakerRes.Decode(&creatorStaker); err != nil {
			return err
		}

		if bet.OverallStatus == models.CreatorWon {
			// Make original bet's loser (bet receiver, in this case) pay out to stake winners
			if err := transferBalance(ctx, receiver, creatorStaker, stake.SharesFilled*bet.CreatorAmount); err != nil {
				return err
			}
		} else {
			// Make stake losers pay out to original bet's loser (stake creator, in this case)
			if err := transferBalance(ctx, creatorStaker, creator, stake.SharesFilled*bet.ReceiverAmount); err != nil {
				return err
			}
		}
	}

	for _, stakeID := range bet.ReceiverStakes {
		stakeRes := stakeCollection.FindOne(ctx, bson.M{"_id": stakeID})
		if stakeRes.Err() != nil {
			return fmt.Errorf("stake id %s not found", stakeID.String())
		}
		var stake models.Stake
		if err := stakeRes.Decode(&stake); err != nil {
			return err
		}

		receiverStakerRes := userCollection.FindOne(ctx, bson.M{"username": stake.OwnerName})
		if creatorRes.Err() != nil {
			return fmt.Errorf("stake winner %s not found", stake.OwnerName)
		}
		var receiverStaker models.User
		if err := receiverStakerRes.Decode(&receiverStaker); err != nil {
			return err
		}

		if bet.OverallStatus == models.CreatorWon {
			// Make stake losers pay out to original bet's loser (stake receiver, in this case)
			if err := transferBalance(ctx, receiverStaker, receiver, stake.SharesFilled*bet.CreatorAmount); err != nil {
				return err
			}
		} else {
			// Make original bet's loser (bet creator, in this case) pay out to stake winners
			if err := transferBalance(ctx, creator, receiverStaker, stake.SharesFilled*bet.ReceiverAmount); err != nil {
				return err
			}
		}
	}

	return nil
}
