package repo

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/Seann-Moser/mserve"
	"go.mongodb.org/mongo-driver/bson"
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

// Mongo is a generic repository implementation for MongoDB.
type Mongo[T any] struct {
	collection *mongo.Collection
}

// NewMongo creates a new Mongo repository instance for a given database and generic type.
func NewMongo[T any](db *mongo.Database) (Repo[T], error) {
	var t T
	collectionName := getStructName(t)
	if collectionName == "" {
		return nil, errors.New("cannot create a Mongo repository for a non-struct type")
	}

	repo := &Mongo[T]{
		collection: db.Collection(collectionName),
	}

	// Create indexes based on struct tags.
	if err := repo.createIndexes(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to create indexes for collection %s: %w", collectionName, err)
	}

	return repo, nil
}

// createIndexes iterates through the struct fields and creates indexes based on "index" and "group" tags.
func (m *Mongo[T]) createIndexes(ctx context.Context) error {
	var t T
	val := reflect.TypeOf(t)

	// Check if the type is a struct, handling pointers.
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return errors.New("cannot create indexes on a non-struct type")
	}

	// Use a map to group keys for compound indexes.
	groupedIndexes := make(map[string]bson.D)

	// Track unique index models to avoid duplicates.
	var indexModels []mongo.IndexModel

	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		bsonTag := field.Tag.Get("bson")
		indexTag := field.Tag.Get("index")
		groupTag := field.Tag.Get("group")

		if indexTag == "" {
			continue
		}

		// Use the bson tag as the key name, if available, otherwise use the field name.
		keyName := field.Name
		if bsonTag != "" {
			// Handle options like `omitempty`.
			parts := strings.Split(bsonTag, ",")
			keyName = parts[0]
		}

		// Parse the index value (e.g., "1", "-1", "text").
		var indexValue int32 = 1
		if indexTag == "1" {
			indexValue = 1
		} else if indexTag == "-1" {
			indexValue = -1
		} else {
			// For other types like "text", keep the string value.
			keyName = "$" + indexTag
			indexValue = 1
		}

		// Handle single-field indexes without a group tag.
		if groupTag == "" {
			indexModels = append(indexModels, mongo.IndexModel{
				Keys: bson.D{{keyName, indexValue}},
			})
			continue
		}

		// Handle grouped indexes.
		keys, ok := groupedIndexes[groupTag]
		if !ok {
			keys = bson.D{}
		}
		keys = append(keys, bson.E{Key: keyName, Value: indexValue})
		groupedIndexes[groupTag] = keys
	}

	// Add the grouped indexes to the list of index models.
	for _, keys := range groupedIndexes {
		indexModels = append(indexModels, mongo.IndexModel{
			Keys: keys,
		})
	}

	if len(indexModels) > 0 {
		_, err := m.collection.Indexes().CreateMany(ctx, indexModels)
		if err != nil {
			return fmt.Errorf("failed to create indexes: %w", err)
		}
	}

	return nil
}

// InsertAny inserts multiple documents of a given type, converting them from `any`.
// This is useful for handling mixed data types.
func (m *Mongo[T]) InsertAny(ctx context.Context, data ...any) ([]T, error) {
	if len(data) == 0 {
		return nil, nil
	}

	insertResult, err := m.collection.InsertMany(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("failed to insert documents: %w", err)
	}

	var insertedDocs []T
	for _, id := range insertResult.InsertedIDs {
		// Use the inserted ID to find the newly created document.
		var doc T
		filter := bson.M{"_id": id}
		err := m.collection.FindOne(ctx, filter).Decode(&doc)
		if err != nil {
			return nil, fmt.Errorf("failed to find inserted document with ID %v: %w", id, err)
		}
		insertedDocs = append(insertedDocs, doc)
	}

	return insertedDocs, nil
}

// Insert inserts multiple documents of type T into the collection.
func (m *Mongo[T]) Insert(ctx context.Context, data ...T) ([]T, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// Create a slice of `any` for the `InsertMany` call.
	docs := make([]any, len(data))
	for i, d := range data {
		docs[i] = d
	}

	insertResult, err := m.collection.InsertMany(ctx, docs)
	if err != nil {
		return nil, fmt.Errorf("failed to insert documents: %w", err)
	}

	var insertedDocs []T
	for _, id := range insertResult.InsertedIDs {
		// Use the inserted ID to find the newly created document.
		var doc T
		filter := bson.M{"_id": id}
		err := m.collection.FindOne(ctx, filter).Decode(&doc)
		if err != nil {
			return nil, fmt.Errorf("failed to find inserted document with ID %v: %w", id, err)
		}
		insertedDocs = append(insertedDocs, doc)
	}

	return insertedDocs, nil
}

// Update updates a single document based on a filter and returns the updated document.
func (m *Mongo[T]) Update(ctx context.Context, update interface{}, filter interface{}, updateOptions *options.UpdateOptions) (T, error) {
	var result T

	// Use FindOneAndUpdate with a `return new` option to get the document after the update.
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	if updateOptions != nil {
		if updateOptions.Upsert != nil {
			opts = opts.SetUpsert(*updateOptions.Upsert)
		}
	}

	err := m.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&result)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return result, fmt.Errorf("no document found to update")
		}
		return result, fmt.Errorf("failed to update document: %w", err)
	}

	return result, nil
}

// List retrieves a paginated list of documents from the collection.
func (m *Mongo[T]) List(ctx context.Context, filter interface{}, findOpts *options.FindOptions, page mserve.Page[T]) (mserve.Page[T], error) {
	// First, get the total count of documents matching the filter.

	return mserve.PaginateMongo[T](ctx, m.collection, filter, page.Page, page.Limit, findOpts)
}

// Delete deletes documents from the collection based on a filter.
func (m *Mongo[T]) Delete(ctx context.Context, filter interface{}) error {
	// DeleteMany is a more generic and powerful option than DeleteOne.
	deleteResult, err := m.collection.DeleteMany(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to delete documents: %w", err)
	}
	slog.Info("deleted documents", "amount", deleteResult.DeletedCount)
	return nil
}
