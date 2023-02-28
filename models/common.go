package models

type RequestStatus int8

const (
	Unchanged RequestStatus = iota
	Accepted
	Declined
	Unfriended
	Blocked
)
