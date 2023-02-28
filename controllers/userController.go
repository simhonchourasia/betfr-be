package controllers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator"
	"github.com/simhonchourasia/betfr-be/authentication"
	"github.com/simhonchourasia/betfr-be/config"
	"github.com/simhonchourasia/betfr-be/database"
	"github.com/simhonchourasia/betfr-be/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

var userCollection *mongo.Collection = database.OpenCollection(database.Client, config.GlobalConfig.UserCollection)
var validate = validator.New()

func HashPassword(password string) string {
	pwdBytes, err := bcrypt.GenerateFromPassword([]byte(password), 10)
	if err != nil {
		log.Panic(err)
	}
	return string(pwdBytes)
}

func VerifyPassword(givenPassword string, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(givenPassword))
	return err == nil
}

// Function to sign up a user
var SignUpFunc gin.HandlerFunc = func(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// put in user data from gin context
	var user models.User
	if err := c.BindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}

	if validationErr := validate.Struct(user); validationErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
		return
	}

	numSameEmail, emailErr := userCollection.CountDocuments(ctx, bson.M{"email": user.Email})
	numSameUsername, usernameErr := userCollection.CountDocuments(ctx, bson.M{"username": user.Username})
	if emailErr != nil || usernameErr != nil {
		err := fmt.Errorf("error when validating username/email: %v; %v", usernameErr, emailErr)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		log.Panic(err)
	}

	if numSameEmail+numSameUsername > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "This username already exists"})
		return
	}
	if numSameEmail > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "This email already exists"})
		return
	}

	password := HashPassword(*user.Password)
	user.Password = &password

	user.ID = primitive.NewObjectID()

	token, refreshToken, err := authentication.GenerateAllTokens(*user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	user.Token = &token
	user.RefreshToken = &refreshToken

	// Initialize everything else
	// TODO: come up with a better solution than raw initializing everything here
	user.OutgoingFriendReqs = make([]string, 0)
	user.IncomingFriendReqs = make([]string, 0)
	user.BlockedUsers = make([]string, 0)
	user.Friends = make([]string, 0)
	user.IncomingBetReqs = make([]primitive.ObjectID, 0)
	user.OutgoingBetReqs = make([]primitive.ObjectID, 0)
	user.ResolvedBets = make([]primitive.ObjectID, 0)
	user.ConflictedBets = make([]primitive.ObjectID, 0)
	user.OngoingBets = make([]primitive.ObjectID, 0)
	user.Balances = make(map[string]int64)
	user.TotalBalance = 0

	res, err := userCollection.InsertOne(ctx, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User signup unsuccessful"})
		return
	}

	c.JSON(http.StatusOK, res)
}

var LoginFunc gin.HandlerFunc = func(c *gin.Context) {
	var ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var user models.User
	var matchingUser models.User

	if err := c.BindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	foundUser := userCollection.FindOne(ctx, bson.M{"email": user.Email})
	if foundUser.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User with email not found"})
		return
	}
	if err := foundUser.Decode(&matchingUser); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	passwordOk := VerifyPassword(*user.Password, *matchingUser.Password)
	if !passwordOk {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Incorrect password"})
	}

	token, refreshToken, err := authentication.GenerateAllTokens(*user.Email)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	authentication.UpdateAllTokens(token, refreshToken, *matchingUser.Username)

	c.JSON(http.StatusOK, matchingUser)
}

var DeleteUserFunc gin.HandlerFunc = func(c *gin.Context) {
	var ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var user models.User

	if err := c.BindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	foundUser, err := userCollection.DeleteOne(ctx, bson.M{"email": user.Email, "username": user.Username})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		log.Panic(err)
	}
	if foundUser.DeletedCount == 0 {
		c.JSON(
			http.StatusInternalServerError,
			gin.H{
				"error": fmt.Sprintf("User %s could not be found for deletion", *user.Username),
			},
		)
		return
	}

	c.JSON(http.StatusOK, gin.H{"msg": fmt.Sprintf("Successfully deleted user %s", *user.Username)})
}

// Helper function to be used in handling friend requests and balance transfers
func UpdateUserHelper(c *gin.Context, ctx context.Context, friendUpdate models.UpdateUserHelperStruct) error {
	upsert := true
	filter := bson.M{"username": friendUpdate.Username}
	opt := options.UpdateOptions{
		Upsert: &upsert,
	}

	var update primitive.M
	if friendUpdate.Operation == "$pullAll" {
		toRemove := []interface{}{friendUpdate.Val}
		update = bson.M{friendUpdate.Field: toRemove}
	} else {
		update = bson.M{friendUpdate.Field: friendUpdate.Val}
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
		return fmt.Errorf("tried to update invalid user %s", friendUpdate.Username)
	}
	return nil
}

// TODO: add support for blocking users with this
// Adds receiver to outgoing friend reqs of sender and adds sender to incoming reqs of receiver
// Note that accepting a friend request doesn't require a previous friend request to be sent (will force friendship)
var SendFriendReqFunc gin.HandlerFunc = func(c *gin.Context) {
	var ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var friendReq models.FriendRequest

	if err := c.BindJSON(&friendReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if *friendReq.Receiver == *friendReq.Sender {
		c.JSON(http.StatusBadRequest, gin.H{"error": "can't send friend request to yourself!"})
		return
	}

	// First check that the users exist
	// TODO: this could be removed? or another endpoint could be added to check if a user exists
	// TODO: maybe only have these really detailed checks for certain checking levels (efficiency vs error handling)
	senderUser := userCollection.FindOne(ctx, bson.M{"username": friendReq.Sender})
	if senderUser.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Friend request sender %s not found", *friendReq.Sender)})
	}
	receiverUser := userCollection.FindOne(ctx, bson.M{"username": friendReq.Receiver})
	if receiverUser.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Friend request receiver %s not found", *friendReq.Receiver)})
		return
	}
	var sender models.User
	if err := senderUser.Decode(&sender); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var receiver models.User
	if err := receiverUser.Decode(&receiver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Sanity check for sender
	for _, v := range sender.Friends {
		if v == *friendReq.Receiver {
			c.JSON(http.StatusBadRequest, gin.H{"error": "users are already friends"})
			return
		}
	}
	for _, v := range sender.IncomingFriendReqs {
		if v == *friendReq.Receiver {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sender already received friend request from receiver"})
			return
		}
	}
	for _, v := range sender.OutgoingFriendReqs {
		if v == *friendReq.Receiver {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sender already sent friend request to receiver"})
			return
		}
	}
	// Sanity check for receiver
	for _, v := range receiver.Friends {
		if v == *friendReq.Sender {
			c.JSON(http.StatusBadRequest, gin.H{"error": "users are already friends"})
			return
		}
	}
	for _, v := range receiver.IncomingFriendReqs {
		if v == *friendReq.Sender {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sender already sent friend request to receiver"})
			return
		}
	}
	for _, v := range receiver.OutgoingFriendReqs {
		if v == *friendReq.Receiver {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sender already received friend request from receiver"})
			return
		}
	}

	// Add to incoming and outgoing lists
	updateSender := models.UpdateUserHelperStruct{
		Username:  *friendReq.Sender,
		Operation: "$push",
		Field:     "outgoingfriendreqs",
		Val:       *friendReq.Receiver,
	}
	if err := UpdateUserHelper(c, ctx, updateSender); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	updateReceiver := models.UpdateUserHelperStruct{
		Username:  *friendReq.Receiver,
		Operation: "$push",
		Field:     "incomingfriendreqs",
		Val:       *friendReq.Sender,
	}
	if err := UpdateUserHelper(c, ctx, updateReceiver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	msg := fmt.Sprintf(
		"Successfully sent friend request from %s to %s",
		*friendReq.Sender,
		*friendReq.Receiver,
	)
	c.JSON(http.StatusOK, gin.H{"msg": msg})
}

// Removes friends from incoming/outgoing friend reqs
// Adds to friends if success
var ResolveFriendReqFunc gin.HandlerFunc = func(c *gin.Context) {
	var ctx, cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var friendReq models.FriendRequest

	if err := c.BindJSON(&friendReq); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if *friendReq.Receiver == *friendReq.Sender {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Can't add yourself as a friend!"})
		return
	}

	// First check that the users exist
	senderUser := userCollection.FindOne(ctx, bson.M{"username": friendReq.Sender})
	if senderUser.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Friend request sender %s not found", *friendReq.Sender)})
		return
	}
	receiverUser := userCollection.FindOne(ctx, bson.M{"username": friendReq.Receiver})
	if receiverUser.Err() != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Friend request receiver %s not found", *friendReq.Receiver)})
		return
	}

	// Ensure that the friend request has already been sent; also that there is indeed a sent friend request between them
	var sender models.User
	if err := senderUser.Decode(&sender); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, friendName := range sender.Friends {
		if friendName == *friendReq.Receiver {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("User %s already in friend list of %s", *friendReq.Receiver, *friendReq.Sender)})
			return
		}
	}
	var receiver models.User
	if err := receiverUser.Decode(&receiver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, friendName := range receiver.Friends {
		if friendName == *friendReq.Sender {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("User %s already in friend list of %s", *friendReq.Sender, *friendReq.Receiver)})
			return
		}
	}

	reqSent := false
	for _, friendName := range sender.OutgoingFriendReqs {
		if friendName == *friendReq.Receiver {
			reqSent = true
		}
	}
	if !reqSent {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("No ongoing request from %s to %s", *friendReq.Sender, *friendReq.Receiver)})
		return
	}
	reqReceived := false
	for _, friendName := range receiver.IncomingFriendReqs {
		if friendName == *friendReq.Sender {
			reqReceived = true
		}
	}
	if !reqReceived {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("No ongoing request to %s from %s", *friendReq.Receiver, *friendReq.Sender)})
		return
	}

	var msg string

	// TODO: put these into separate functions
	if *friendReq.ReqStatus == models.Unchanged {
		msg = fmt.Sprintf("Unchanged friend status between %s and %s", *friendReq.Sender, *friendReq.Receiver)
	} else if *friendReq.ReqStatus == models.Unfriended {
		updateSender := models.UpdateUserHelperStruct{
			Username:  *friendReq.Sender,
			Operation: "$pullAll",
			Field:     "friends",
			Val:       *friendReq.Receiver,
		}
		if err := UpdateUserHelper(c, ctx, updateSender); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updateReceiver := models.UpdateUserHelperStruct{
			Username:  *friendReq.Receiver,
			Operation: "$pullAll",
			Field:     "friends",
			Val:       *friendReq.Sender,
		}
		if err := UpdateUserHelper(c, ctx, updateReceiver); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		msg = fmt.Sprintf("Unfriended %s and %s", *friendReq.Sender, *friendReq.Receiver)
	} else {
		// Otherwise we are just accepting a friend request
		// First remove from incoming and outgoing lists
		updateSender := models.UpdateUserHelperStruct{
			Username:  *friendReq.Sender,
			Operation: "$pullAll",
			Field:     "outgoingfriendreqs",
			Val:       *friendReq.Receiver,
		}
		if err := UpdateUserHelper(c, ctx, updateSender); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updateReceiver := models.UpdateUserHelperStruct{
			Username:  *friendReq.Receiver,
			Operation: "$pullAll",
			Field:     "outgoingfriendreqs",
			Val:       *friendReq.Sender,
		}
		if err := UpdateUserHelper(c, ctx, updateReceiver); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updateSender = models.UpdateUserHelperStruct{
			Username:  *friendReq.Sender,
			Operation: "$pullAll",
			Field:     "incomingfriendreqs",
			Val:       *friendReq.Receiver,
		}
		if err := UpdateUserHelper(c, ctx, updateSender); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updateReceiver = models.UpdateUserHelperStruct{
			Username:  *friendReq.Receiver,
			Operation: "$pullAll",
			Field:     "incomingfriendreqs",
			Val:       *friendReq.Sender,
		}
		if err := UpdateUserHelper(c, ctx, updateReceiver); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if *friendReq.ReqStatus == models.Accepted {
			// Then add to friends
			updateSender = models.UpdateUserHelperStruct{
				Username:  *friendReq.Sender,
				Operation: "$push",
				Field:     "friends",
				Val:       *friendReq.Receiver,
			}
			if err := UpdateUserHelper(c, ctx, updateSender); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			updateReceiver = models.UpdateUserHelperStruct{
				Username:  *friendReq.Receiver,
				Operation: "$push",
				Field:     "friends",
				Val:       *friendReq.Sender,
			}
			if err := UpdateUserHelper(c, ctx, updateReceiver); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			msg = fmt.Sprintf("Added %s and %s as friends", *friendReq.Sender, *friendReq.Receiver)
		} else if *friendReq.ReqStatus == models.Declined {
			msg = fmt.Sprintf("Declined friend request between %s and %s", *friendReq.Sender, *friendReq.Receiver)
		}

	}

	c.JSON(http.StatusOK, gin.H{"msg": msg})
}

// Helper function to transfer a balance from one user to another
func transferBalance(ctx context.Context, loser models.User, winner models.User, amount int64) error {
	upsert := true
	opt := options.UpdateOptions{
		Upsert: &upsert,
	}

	newWinnerBalance := winner.Balances[*loser.Username] + amount

	winnerRes, err := userCollection.UpdateOne(
		ctx,
		bson.M{"username": *winner.Username},
		bson.D{
			{
				Key:   "$set",
				Value: bson.M{fmt.Sprintf("balances.%s", *loser.Username): newWinnerBalance},
			},
			{
				Key:   "$set",
				Value: bson.M{"totalbalance": winner.TotalBalance + amount},
			},
		},
		&opt,
	)
	if err != nil {
		log.Panic(err)
		return err
	}
	if winnerRes.MatchedCount == 0 {
		return fmt.Errorf("tried to handle balance for invalid user %s", *winner.Username)
	}

	newLoserBalance := loser.Balances[*winner.Username] - amount
	loserRes, err := userCollection.UpdateOne(
		ctx,
		bson.M{"username": *winner.Username},
		bson.D{
			{
				Key:   "$set",
				Value: bson.M{fmt.Sprintf("balances.%s", *loser.Username): newLoserBalance},
			},
			{
				Key:   "$set",
				Value: bson.M{"totalbalance": loser.TotalBalance - amount},
			},
		},
		&opt,
	)
	if err != nil {
		log.Panic(err)
		return err
	}
	if loserRes.MatchedCount == 0 {
		return fmt.Errorf("tried to handle balance for invalid user %s", *winner.Username)
	}

	return nil
}
