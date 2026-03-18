package domain

import "errors"

var ErrNotFound = errors.New("can not find zipcode")
var ErrInvalidCEP = errors.New("invalid zipcode")
