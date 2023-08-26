package models

import "go.mongodb.org/mongo-driver/bson/primitive"

type User struct {
	ID                 primitive.ObjectID   `bson:"_id,omitempty"`
	Username           *string              `json:"username" validate:"required,min=1,max=30"`
	Email              *string              `json:"email" validate:"email,required"`
	Password           *string              `json:"password" validate:"required,min=6,max=100"`
	Token              *string              `json:"token"`
	RefreshToken       *string              `json:"refreshtoken"`
	OutgoingFriendReqs []string             `json:"outgoingfriendreqs"`
	IncomingFriendReqs []string             `json:"incomingfriendreqs"`
	BlockedUsers       []string             `json:"blockedusers"`
	Friends            []string             `json:"friends"`
	IncomingBetReqs    []primitive.ObjectID `json:"incomingbetreqs"`
	OutgoingBetReqs    []primitive.ObjectID `json:"outgoingbetreqs"`
	ResolvedBets       []primitive.ObjectID `json:"resolvedbets"`
	ConflictedBets     []primitive.ObjectID `json:"conflictedbets"`
	OngoingBets        []primitive.ObjectID `json:"ongoingbets"`
	ResolvedStakes     []primitive.ObjectID `json:"resolvedstakes"`
	OngoingStakes      []primitive.ObjectID `json:"ongoingstakes"`
	Balances           map[string]int64     `json:"balances"`
	TotalBalance       int64                `json:"totalbalance"`
	NumBets            int                  `json:"numbets"`
}

type UpdateUserHelperStruct struct {
	Username  string
	Operation string
	Field     string
	Val       string
	IdVal     primitive.ObjectID
}

type FriendRequest struct {
	Sender    *string        `json:"sender" validate:"required,min=1,max=30"`
	Receiver  *string        `json:"receiver" validate:"required,min=1,max=30"`
	ReqStatus *RequestStatus `json:"friendreqstatus"`
}
