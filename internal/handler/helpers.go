package handler

import "github.com/tonypk/aistarlight-go/pkg/pagination"

func defaultPagination() pagination.Params {
	return pagination.Params{Page: 1, Limit: 100, Offset: 0}
}
