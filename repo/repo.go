package repo

import (
	"context"
	"reflect"

	"github.com/DarlingGoose/mserve"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Repo defines the generic interface for a repository.
type Repo[T any] interface {
	InsertAny(ctx context.Context, data ...any) ([]T, error)
	Insert(ctx context.Context, data ...T) ([]T, error)
	Update(ctx context.Context, update interface{}, filter interface{}, updateOptions *options.UpdateOptions) (T, error)

	List(ctx context.Context, filter interface{}, findOpts *options.FindOptions, page mserve.Page[T]) (mserve.Page[T], error)
	Delete(ctx context.Context, filter interface{}) error
	Aggregate(ctx context.Context, pipeline mongo.Pipeline) (*mongo.Cursor, error)
	Mongo() *mongo.Collection
}

// getStructName takes a variable as an interface and returns the name of its struct type.
// It handles both struct values and pointers to structs.
// If the variable is not a struct, it returns an empty string.
func getStructName(v interface{}) string {
	// Get the reflect.Type of the variable.
	// This will give us the type, whether it's a pointer or a struct itself.
	t := reflect.TypeOf(v)

	// Check if the type is a pointer. If it is, get the underlying element type.
	// For example, from "*main.User" to "main.User".
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Now check if the underlying type is a struct.
	// If it is, return its name.
	if t.Kind() == reflect.Struct {
		return t.Name()
	}

	// If it's not a struct, return an empty string.
	return ""
}
