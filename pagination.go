package mserve

import (
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log/slog"
	"math"
	"net/http"
	"strconv"
)

type Page[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

// Paginate slices `items` according to page & limit.
// Returns a Page[T] or an error if page/limit are invalid.
func Paginate[T any](items []T, page, limit int) (Page[T], error) {
	if page < 1 || limit < 1 {
		return Page[T]{}, errors.New("page and limit must be >= 1")
	}

	total := len(items)
	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	// clamp start/end
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	return Page[T]{
		Items:      items[start:end],
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

// PaginateMongo runs a MongoDB query with skip/limit and returns a Page[T].
// - ctx: request context
// - coll: the collection to query
// - filter: any BSON filter (e.g. bson.M{"status":"active"})
// - page, limit: pagination parameters (must be >=1)
// - findOpts: any extra FindOptions (e.g. Sort)
// Returns Err if page/limit <1 or Mongo errors.
// PaginateMongo runs a MongoDB query with skip/limit and returns a Page[T].
// - ctx: request context
// - coll: the collection to query
// - filter: any BSON filter (e.g. bson.M{"status":"active"})
// - page, limit: pagination parameters (must be >=1)
// - findOpts: a single FindOptions (e.g. options.Find().SetSort(...)), or nil
// Returns an error if page/limit <1 or if any Mongo call fails.
func PaginateMongo[T any](
	ctx context.Context,
	coll *mongo.Collection,
	filter interface{},
	page, limit int,
	findOpts *options.FindOptions,
) (Page[T], error) {
	if page < 1 || limit < 1 {
		return Page[T]{}, errors.New("page and limit must be >= 1")
	}

	// 1. total count
	total64, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return Page[T]{}, err
	}
	total := int(total64)
	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	// 2. ensure we have a FindOptions to modify
	opts := findOpts
	if opts == nil {
		opts = options.Find()
	}
	opts.SetSkip(int64((page - 1) * limit))
	opts.SetLimit(int64(limit))

	// 3. execute query
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return Page[T]{}, err
	}
	defer func(cursor *mongo.Cursor, ctx context.Context) {
		err := cursor.Close(ctx)
		if err != nil {
			slog.Error("failed to close cursor", "err", err)
		}
	}(cursor, ctx)

	// 4. decode results
	var items []T
	if err := cursor.All(ctx, &items); err != nil {
		return Page[T]{}, err
	}

	// 5. build page response
	return Page[T]{
		Items:      items,
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

// QueryParams extracts page & limit from the URL, with defaults.
func QueryParams(r *http.Request, defaultLimit int) (page, limit int) {
	q := r.URL.Query()
	page = parseOr(q.Get("page"), 1)
	limit = parseOr(q.Get("limit"), defaultLimit)
	return
}

func parseOr(s string, d int) int {
	if v, err := strconv.Atoi(s); err == nil && v > 0 {
		return v
	}
	return d
}
