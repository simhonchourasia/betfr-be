package authentication

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/simhonchourasia/betfr-be/config"
	"github.com/simhonchourasia/betfr-be/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var userCollection *mongo.Collection = database.OpenCollection(database.Client, config.GlobalConfig.UserCollection)

// Used to hold JWT info
type SignedDetails struct {
	Username string
	jwt.StandardClaims
}

func ValidateToken(signedToken string) (*SignedDetails, error) {
	token, err := jwt.ParseWithClaims(
		signedToken,
		&SignedDetails{},
		func(token *jwt.Token) (interface{}, error) {
			return []byte(config.GlobalConfig.SecretKey), nil
		},
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*SignedDetails)
	if !ok {
		return nil, fmt.Errorf("invalid token")
	}

	if claims.ExpiresAt < time.Now().Local().Unix() {
		return nil, fmt.Errorf("expired token")
	}

	return claims, nil
}

func GenerateAllTokens(username string) (string, string, error) {
	expiryHours := 24
	if config.GlobalConfig.Debug {
		expiryHours = 168
	}
	claims := &SignedDetails{
		Username: username,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Local().Add(time.Duration(expiryHours) * time.Hour).Unix(),
		},
	}

	refreshClaims := &SignedDetails{
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Local().Add(time.Duration(24) * time.Duration(7) * time.Hour).Unix(),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.GlobalConfig.SecretKey))
	refreshToken, err2 := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(config.GlobalConfig.SecretKey))
	log.Printf("Generated JWT tokens for %s", username)
	if err != nil || err2 != nil {
		err = fmt.Errorf("%v %v", err, err2)
		log.Panic(err)
		return "", "", err
	}

	return token, refreshToken, err
}

// Uses only username
func UpdateAllTokens(signedToken string, signedRefreshToken string, username string) {
	var ctx, cancel = context.WithTimeout(context.Background(), time.Duration(2)*time.Minute)
	defer cancel()

	var updateObj primitive.D
	updateObj = append(updateObj, bson.E{Key: "token", Value: signedToken}, bson.E{Key: "refresh_token", Value: signedRefreshToken})

	upsert := true
	filter := bson.M{"username": username}
	opt := options.UpdateOptions{
		Upsert: &upsert,
	}
	_, err := userCollection.UpdateOne(
		ctx,
		filter,
		bson.D{
			{Key: "$set", Value: updateObj},
		},
		&opt,
	)

	if err != nil {
		log.Panic(err)
		return
	}
}
