package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type BetStatus int8
type BetRequestStatus int8

const (
	Undecided BetStatus = 0
	CreatorWon
	ReceiverWon
	Conflicted
)

type Bet struct {
	ID                     primitive.ObjectID   `bson:"_id,omitempty"`
	BetID                  *string              `json:"betid"` // concatenates username with bet number
	OverallStatus          BetStatus            `json:"overallstatus"`
	CreatorName            string               `bson:"creatorname"`
	ReceiverName           string               `bson:"receivername"`
	CreatorAmount          int64                `json:"creatoramount"`  // numerator of ratio
	ReceiverAmount         int64                `json:"receiveramount"` // denominator of ratio
	NumShares              int64                `json:"numshares"`      // see comments at bottom for more info
	CreatorStatus          BetStatus            `json:"creatorstatus"`
	ReceiverStatus         BetStatus            `json:"receiverstatus"`
	CreatorStaked          int64                `json:"creatorstaked"`  // number stakes in favour of creator
	ReceiverStaked         int64                `json:"receiverstaked"` // number stakes in favour of receiver
	CreatorStakedUnfilled  int64                `json:"creatorstakedunfilled"`
	ReceiverStakedUnfilled int64                `json:"receiverstakedunfilled"`
	CreatorStakes          []primitive.ObjectID `json:"creatorstakes"` // queues of stakes; earlier stuff gets filled first
	ReceiverStakes         []primitive.ObjectID `json:"receiverstakes"`
	Underlying             *string              `json:"underlying"` // uses BetID
	Title                  string               `json:"title"`
	Description            string               `json:"description"`
	CreateDate             primitive.DateTime   `json:"createdate"`
	ExpiryDate             primitive.DateTime   `json:"expirydate"`
}

// CreatorAmount and ReceiverAmount are just betting odds
// NumShares is the number of times it is duplicated
// note that people can only deal in integer multiples of tokens
// Ex. CreatorAmount is 10, ReceiverAmount is 3, with NumShares being 10
// If Creator wins, Receiver gives them 100 tokens
// If Receiver wins, Creator gives them 30 tokens
// Stakeholders on the Creator's side must stake tokens in multiples of 3
// Stakeholders on the Receiver's side must stake tokens in multiples of 10
// i.e. every 3 tokens staked on creator's side will be matched with 10 tokens staked for receiver
// Note that the CreatorAmount and ReceiverAmount also act as minimum amounts that must be invested
// i.e. they decide the stake price multiples if they specify NumShares
// If NumShares is not given, we will divide CreatorAmount and ReceiverAmount by their GCD
// and set NumShares to their gcd

type BetReqHandle struct {
	BetID        primitive.ObjectID `json:"betid"`
	BetReqStatus RequestStatus      `json:"betreqstatus"`
}

type BetResolve struct {
	BetID            primitive.ObjectID `json:"betid"`
	BetResolveStatus BetStatus          `json:"betresolvestatus"`
	Username         string             `json:"username"`
}
