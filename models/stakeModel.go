package models

import "go.mongodb.org/mongo-driver/bson/primitive"

// These get added to a queue for each bet until they are completely filled
// When partially or completely filled, they are added to a set in the underlying bet
type Stake struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	Underlying     primitive.ObjectID `json:"underlying"`
	OwnerName      string             `json:"ownername"`
	SharesStaked   int64              `bson:"amountstaked"`
	SharesFilled   int64              `bson:"amountfilled"`
	BackingCreator bool               `json:"backingcreator"`
	Comment        string             `json:"comment"`
	CreateDate     primitive.DateTime `json:"createdate"`
}

type StakeRequest struct {
	Underlying     primitive.ObjectID `json:"underlying"`
	OwnerName      string             `json:"ownername"`
	NumShares      int64              `bson:"numshares"`
	BackingCreator bool               `json:"backingcreator"`
	Comment        string             `json:"comment"`
}

// UNUSED
type StakeUpdate struct {
	Underlying primitive.ObjectID `json:"underlying"`
	CreatorWon bool               `json:"creatorwon"`
}
