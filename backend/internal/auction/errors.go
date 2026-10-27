package auction

import "errors"

var (
	ErrLotNotActive    = errors.New("lot is not active")
	ErrSellerCannotBid = errors.New("seller cannot bid on their own lot")
	ErrBidTooLow       = errors.New("bid must be at least current price plus the minimum step")
)
