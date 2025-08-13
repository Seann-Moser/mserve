package extract

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gopkg.in/yaml.v3"
)

// ExtractionRule describes how to extract one or more fields.
type ExtractionRule struct {
	Name       string            `json:"name" yaml:"name" bson:"name"`
	Selector   string            `json:"selector" yaml:"selector" bson:"selector"`
	Attr       string            `json:"attr,omitempty" yaml:"attr,omitempty" bson:"attr,omitempty"`
	Multiple   bool              `json:"multiple,omitempty" yaml:"multiple,omitempty" bson:"multiple,omitempty"`
	Download   bool              `json:"download,omitempty" yaml:"download,omitempty" bson:"download,omitempty"`
	Visit      bool              `json:"visit" yaml:"visit" bson:"visit"`
	Flatten    bool              `json:"flatten,omitempty" yaml:"flatten,omitempty" bson:"flatten,omitempty"`
	SaveDir    string            `json:"save_dir,omitempty" yaml:"save_dir,omitempty" bson:"save_dir,omitempty"`
	Children   []*ExtractionRule `json:"children,omitempty" yaml:"children,omitempty" bson:"children,omitempty"`
	Transforms []*Transforms     `json:"transforms,omitempty" yaml:"transforms,omitempty" bson:"transforms,omitempty"`
}
type Transforms struct {
	Match   string `json:"match"`
	Split   bool   `json:"split"`
	Replace string `json:"replace"`
}

// SaveRulesToJSON writes rules as a single JSON array.
func SaveRulesToJSON(rules []*ExtractionRule, path string) error {
	data, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadRulesFromJSON reads a JSON file into rules.
func LoadRulesFromJSON(path string) ([]*ExtractionRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rules []*ExtractionRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// SaveRulesToYAML writes rules as a YAML document.
func SaveRulesToYAML(rules []*ExtractionRule, path string) error {
	data, err := yaml.Marshal(rules)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadRulesFromYAML reads a YAML file into rules.
func LoadRulesFromYAML(path string) ([]*ExtractionRule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rules []*ExtractionRule
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

// LoadRulesFromDir walks dirPath and loads any JSON/YAML rule files it finds.
func LoadRulesFromDir(dirPath string) ([]*ExtractionRule, error) {
	var all []*ExtractionRule
	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch ext := filepath.Ext(path); ext {
		case ".json":
			rs, err := LoadRulesFromJSON(path)
			if err != nil {
				return fmt.Errorf("json %s: %w", path, err)
			}
			all = append(all, rs...)
		case ".yaml", ".yml":
			rs, err := LoadRulesFromYAML(path)
			if err != nil {
				return fmt.Errorf("yaml %s: %w", path, err)
			}
			all = append(all, rs...)
		}
		return nil
	})
	return all, err
}

// SaveRulesToMongo stores rules under a single document with given key.
func SaveRulesToMongo(ctx context.Context, client *mongo.Client, db, coll, key string, rules []*ExtractionRule) error {
	c := client.Database(db).Collection(coll)
	doc := bson.M{
		"_id":   key,
		"rules": rules,
		"ts":    time.Now(),
	}
	// upsert by key
	_, err := c.UpdateOne(ctx,
		bson.M{"_id": key},
		bson.M{"$set": doc},
		options.Update().SetUpsert(true),
	)
	return err
}

// LoadRulesFromMongo fetches the rules document by key.
func LoadRulesFromMongo(ctx context.Context, client *mongo.Client, db, coll, key string) ([]*ExtractionRule, error) {
	c := client.Database(db).Collection(coll)
	var result struct {
		Rules []*ExtractionRule `bson:"rules"`
	}
	err := c.FindOne(ctx, bson.M{"_id": key}).Decode(&result)
	if err != nil {
		return nil, err
	}
	return result.Rules, nil
}
