package router

import "strconv"

func itoa(id uint) string { return strconv.FormatUint(uint64(id), 10) }
