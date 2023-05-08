package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/simhonchourasia/betfr-be/authentication"
	"github.com/simhonchourasia/betfr-be/config"
	"github.com/simhonchourasia/betfr-be/database"
	"github.com/simhonchourasia/betfr-be/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var betCollection *mongo.Collection = database.OpenCollection(database.Client, config.GlobalConfig.BetCollection)

// Pass in creator name, receiver name, creator amount, receiver amount, underlying, title, description, expiry date
// Underlying can also be passed in, if appropriate
var CreateBetReqFunc gin.HandlerFunc = func(c *gin.Context) {
	var ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var bet models.Bet

	if err := c.BindJSON(&bet); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check that the user sending the bet request is the one logged in
	if permissionErr := authentication.CheckUserPermissions(c, &bet.CreatorName); permissionErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": permissionErr.Error()})
		return
	}

	fmt.Printf("\naslkdfjlk\n")

	fmt.Printf("%v\n", bet)

	if validationErr := validate.Struct(bet); validationErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
		return
	}

	update := bson.D{
		{Key: "$inc", Value: bson.M{"numbets": 1}},
	}
	creatorUser := userCollection.FindOneAndUpdate(ctx, bson.M{"username": bet.CreatorName}, update)
	if creatorUser.Err() != nil {
		fmt.Printf("%s\n", creatorUser.Err().Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Bet creator %s not found", bet.CreatorName)})
		return
	}
	receiverUser := userCollection.FindOneAndUpdate(ctx, bson.M{"username": bet.ReceiverName}, update)
	if receiverUser.Err() != nil {
		fmt.Printf("%s\n", receiverUser.Err().Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Bet receiver %s not found", bet.ReceiverName)})
		return
	}

	var creator models.User
	if err := creatorUser.Decode(&creator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var receiver models.User
	if err := receiverUser.Decode(&receiver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	bet.ID = primitive.NewObjectID()
	betID := fmt.Sprintf("%s.%s.%d.%d", bet.CreatorName, bet.ReceiverName, creator.NumBets, receiver.NumBets)
	bet.BetID = &betID
	bet.OverallStatus = models.Undecided
	bet.CreatorStatus = models.Undecided
	bet.ReceiverStatus = models.Undecided
	bet.CreatorStaked = 0
	bet.ReceiverStaked = 0
	if bet.NumShares == 0 {
		bet.NumShares = 10
	}
	bet.CreatorStakedUnfilled = 0
	bet.ReceiverStakedUnfilled = 0
	bet.CreatorStakes = make([]primitive.ObjectID, 0)
	bet.ReceiverStakes = make([]primitive.ObjectID, 0)
	bet.CreateDate = primitive.NewDateTimeFromTime(time.Now())

	if bet.ExpiryDate.Time().Before(bet.CreateDate.Time().Add(5 * time.Minute)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Bets cannot be created with less than 5 minutes to expiry upon creation"})
	}

	res, err := betCollection.InsertOne(ctx, bet)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Bet creation unsuccessful"})
		return
	}

	betObjectId := (res.InsertedID).(primitive.ObjectID)
	updateCreatorBet := bson.D{
		{Key: "$push", Value: bson.M{"outgoingbetreqs": betObjectId}},
	}
	updateReceiverBet := bson.D{
		{Key: "$push", Value: bson.M{"incomingbetreqs": betObjectId}},
	}
	creatorUser = userCollection.FindOneAndUpdate(ctx, bson.M{"username": bet.CreatorName}, updateCreatorBet)
	if creatorUser.Err() != nil {
		fmt.Printf("%s\n", creatorUser.Err().Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Bet creator %s not found", bet.CreatorName)})
		return
	}
	receiverUser = userCollection.FindOneAndUpdate(ctx, bson.M{"username": bet.ReceiverName}, updateReceiverBet)
	if receiverUser.Err() != nil {
		fmt.Printf("%s\n", receiverUser.Err().Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Bet receiver %s not found", bet.ReceiverName)})
		return
	}

	c.JSON(http.StatusOK, res)
}

// Helper function to be used in handling bet requests
func UpdateBetHelper(ctx context.Context, friendUpdate models.UpdateUserHelperStruct) error {
	fmt.Printf("update bet: %v\n", friendUpdate)
	upsert := true
	filter := bson.M{"username": friendUpdate.Username}
	opt := options.UpdateOptions{
		Upsert: &upsert,
	}

	var update primitive.M
	if friendUpdate.Operation == "$pullAll" {
		toRemove := []interface{}{friendUpdate.IdVal}
		update = bson.M{friendUpdate.Field: toRemove}
	} else {
		update = bson.M{friendUpdate.Field: friendUpdate.IdVal}
	}

	res, err := userCollection.UpdateOne(
		ctx,
		filter,
		bson.D{{Key: friendUpdate.Operation, Value: update}},
		&opt,
	)

	if err != nil {
		log.Panic(err)
		return err
	}

	if res.MatchedCount == 0 {
		return fmt.Errorf("tried to handle friend request for invalid user %s", friendUpdate.Username)
	}
	return nil
}

var HandleBetReqFunc gin.HandlerFunc = func(c *gin.Context) {
	var ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var betReqHandle models.BetReqHandle

	if err := c.BindJSON(&betReqHandle); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	fmt.Printf("%v\n", betReqHandle)

	if validationErr := validate.Struct(betReqHandle); validationErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
		return
	}

	betId := betReqHandle.BetID
	betResult := betCollection.FindOne(ctx, bson.M{"_id": betId})
	var bet models.Bet
	if betResult.Err() != nil {
		fmt.Println(betResult.Err().Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Bet ID %s not found", betId.String())})
		return
	}
	if err := betResult.Decode(&bet); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Check that the user accepting the bet request is the one logged in
	if permissionErr := authentication.CheckUserPermissions(c, &bet.ReceiverName); permissionErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": permissionErr.Error()})
		return
	}

	var msg string

	if betReqHandle.BetReqStatus == models.Unchanged {
		msg = fmt.Sprintf("Unchanged bet: %s", bet.Title)
		c.JSON(http.StatusOK, gin.H{"msg": msg})
		return
	}
	if betReqHandle.BetReqStatus != models.Accepted && betReqHandle.BetReqStatus != models.Declined {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad bet req status used"})
		return
	}

	// first check that the bet request is in the outgoing of creator and incoming of receiver
	creatorRes := userCollection.FindOne(ctx, bson.M{"username": bet.CreatorName})
	if creatorRes.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Bet creator %s not found", bet.CreatorName)})
	}
	receiverRes := userCollection.FindOne(ctx, bson.M{"username": bet.ReceiverName})
	if receiverRes.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Bet creator %s not found", bet.ReceiverName)})
		return
	}
	var creator models.User
	if err := creatorRes.Decode(&creator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var receiver models.User
	if err := receiverRes.Decode(&receiver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, v := range creator.OngoingBets {
		if v == betId {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bet is already ongoing"})
			return
		}
	}
	for _, v := range receiver.OngoingBets {
		if v == betId {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bet is already ongoingr"})
			return
		}
	}
	sent := false
	received := false
	for _, v := range creator.OutgoingBetReqs {
		if v == betId {
			sent = true
			break
		}
	}
	if !sent {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trying to handle bet request that was not sent"})
		return
	}
	for _, v := range receiver.IncomingBetReqs {
		if v == betId {
			received = true
			break
		}
	}
	if !received {
		c.JSON(http.StatusBadRequest, gin.H{"error": "trying to handle bet request that was not received"})
		return
	}

	// after checking, remove from the creator and receiver incoming/outgoing bet reqs
	updateCreator := models.UpdateUserHelperStruct{
		Username:  bet.CreatorName,
		Operation: "$pullAll",
		Field:     "outgoingbetreqs",
		IdVal:     betId,
	}
	if err := UpdateBetHelper(ctx, updateCreator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updateReceiver := models.UpdateUserHelperStruct{
		Username:  bet.ReceiverName,
		Operation: "$pullAll",
		Field:     "incomingbetreqs",
		IdVal:     betId,
	}
	if err := UpdateBetHelper(ctx, updateReceiver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Then, if accepted, add to ongoing bets
	// (the check that the bet hasn't been set with an expiry date in the past should be in creation)
	if betReqHandle.BetReqStatus == models.Accepted {
		updateCreator = models.UpdateUserHelperStruct{
			Username:  bet.CreatorName,
			Operation: "$push",
			Field:     "ongoingbets",
			IdVal:     betId,
		}
		if err := UpdateBetHelper(ctx, updateCreator); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updateReceiver = models.UpdateUserHelperStruct{
			Username:  bet.ReceiverName,
			Operation: "$push",
			Field:     "ongoingbets",
			IdVal:     betId,
		}
		if err := UpdateBetHelper(ctx, updateReceiver); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		msg = fmt.Sprintf("Added bet between %s and %s", bet.CreatorName, bet.ReceiverName)
	} else if betReqHandle.BetReqStatus == models.Declined {
		msg = fmt.Sprintf("Declined bet request between %s and %s", bet.CreatorName, bet.ReceiverName)
	}

	c.JSON(http.StatusOK, gin.H{"msg": msg})
}

var ResolveBetFunc gin.HandlerFunc = func(c *gin.Context) {
	var ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	msg := "ok"
	var betResolve models.BetResolve

	if err := c.BindJSON(&betResolve); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	fmt.Printf("%v\n", betResolve)

	// Check that the user modifying the bet request is the one logged in
	if permissionErr := authentication.CheckUserPermissions(c, &betResolve.Username); permissionErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": permissionErr.Error()})
		fmt.Printf("bet request modifier is not the one logged in\n")
		return
	}

	if validationErr := validate.Struct(betResolve); validationErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
		return
	}

	betResult := betCollection.FindOne(ctx, bson.M{"_id": betResolve.BetID})
	var bet models.Bet
	if betResult.Err() != nil {
		fmt.Println(betResult.Err().Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Bet ID %s not found", betResolve.BetID.String())})
		return
	}
	if err := betResult.Decode(&bet); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Ensure that only one of the two members of the bet can provide updates for it
	creatorPermissible := authentication.CheckUserPermissions(c, &bet.CreatorName)
	receiverPermissible := authentication.CheckUserPermissions(c, &bet.ReceiverName)
	if creatorPermissible != nil && receiverPermissible != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": creatorPermissible.Error()})
		fmt.Printf("logged in user is not one of the bettors\n")
		return
	}

	if bet.CreatorName != betResolve.Username && bet.ReceiverName != betResolve.Username {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only creator or receiver can provide a resolve update"})
		return
	}
	// First make sure that the bet isn't already resolved (can't change a resolved bet)
	creatorRes := userCollection.FindOne(ctx, bson.M{"username": bet.CreatorName})
	if creatorRes.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Bet creator %s not found", bet.CreatorName)})
	}
	receiverRes := userCollection.FindOne(ctx, bson.M{"username": bet.ReceiverName})
	if receiverRes.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Bet creator %s not found", bet.ReceiverName)})
		return
	}
	var creator models.User
	if err := creatorRes.Decode(&creator); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var receiver models.User
	if err := receiverRes.Decode(&receiver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, v := range creator.ResolvedBets {
		if v == bet.ID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bet is already resolved"})
			return
		}
	}
	for _, v := range receiver.ResolvedBets {
		if v == bet.ID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bet is already resolved"})
			return
		}
	}
	// Assume then that the bet is ongoing or conflicted

	// In any case, update the CreatorStatus/ReceiverStatus
	bothStatusDecided := false
	if betResolve.Username == bet.CreatorName {
		bet.CreatorStatus = betResolve.BetResolveStatus
		if bet.ReceiverStatus != models.Undecided {
			bothStatusDecided = true
		}
	}
	if betResolve.Username == bet.ReceiverName {
		bet.ReceiverStatus = betResolve.BetResolveStatus
		if bet.CreatorStatus != models.Undecided {
			bothStatusDecided = true
		}
	}
	log.Printf("Both status decided for bet resolve: %t\n", bothStatusDecided)
	if bothStatusDecided {
		if bet.ReceiverStatus != bet.CreatorStatus {
			bet.OverallStatus = models.Conflicted
		} else {
			// bet is fully resolved
			bet.OverallStatus = bet.CreatorStatus
		}
		// Either way, remove from ongoing bets (either conflicted or resolved)
		// Remove from ongoing bets
		updateCreator := models.UpdateUserHelperStruct{
			Username:  bet.CreatorName,
			Operation: "$pullAll",
			Field:     "ongoingbets",
			IdVal:     bet.ID,
		}
		if err := UpdateBetHelper(ctx, updateCreator); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updateReceiver := models.UpdateUserHelperStruct{
			Username:  bet.ReceiverName,
			Operation: "$pullAll",
			Field:     "ongoingbets",
			IdVal:     bet.ID,
		}
		if err := UpdateBetHelper(ctx, updateReceiver); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// If the other person already provided a status, and if they match, move the bet to the resolved list and change balances
	if bet.OverallStatus == models.CreatorWon || bet.OverallStatus == models.ReceiverWon {
		// Add to resolved bets
		updateCreator := models.UpdateUserHelperStruct{
			Username:  bet.CreatorName,
			Operation: "$push",
			Field:     "resolvedbets",
			IdVal:     bet.ID,
		}
		if err := UpdateBetHelper(ctx, updateCreator); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updateReceiver := models.UpdateUserHelperStruct{
			Username:  bet.ReceiverName,
			Operation: "$push",
			Field:     "resolvedbets",
			IdVal:     bet.ID,
		}
		if err := UpdateBetHelper(ctx, updateReceiver); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Handle balances for the winner and loser
		var balanceErr error
		if bet.OverallStatus == models.CreatorWon {
			balanceErr = transferBalance(ctx, receiver, creator, bet.CreatorAmount*bet.NumShares)
		} else if bet.OverallStatus == models.ReceiverWon {
			balanceErr = transferBalance(ctx, creator, receiver, bet.ReceiverAmount*bet.NumShares)
		}
		if balanceErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": balanceErr.Error()})
			return
		}

		// Go over stakes and change balances accordingly
		if err := PayoutStakes(ctx, &bet); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		msg = fmt.Sprintf("Resolved bet between %s and %s", bet.CreatorName, bet.ReceiverName)
	}

	// if the other person already provided a status, and if they don't match, move to the conflicted list
	if bet.OverallStatus == models.Conflicted {
		// Add to conflicted bets
		updateCreator := models.UpdateUserHelperStruct{
			Username:  bet.CreatorName,
			Operation: "$push",
			Field:     "conflictedbets",
			IdVal:     bet.ID,
		}
		if err := UpdateBetHelper(ctx, updateCreator); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updateReceiver := models.UpdateUserHelperStruct{
			Username:  bet.ReceiverName,
			Operation: "$push",
			Field:     "conflictedbets",
			IdVal:     bet.ID,
		}
		if err := UpdateBetHelper(ctx, updateReceiver); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		msg = fmt.Sprintf("Conflicted bet between %s and %s", bet.CreatorName, bet.ReceiverName)
	}

	// Finally, update the bet itself
	opts := options.Replace().SetUpsert(true)
	filter := bson.M{"_id": bet.ID}

	res, err := betCollection.ReplaceOne(
		ctx,
		filter,
		bet,
		opts,
	)
	if err != nil || res.MatchedCount == 0 {
		log.Printf("Could not update bet when trying to resolve\n")
		if err == nil {
			err = fmt.Errorf("bet did not previously exist when trying to update/replace")
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"msg": msg})
}
