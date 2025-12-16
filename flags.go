package mserve

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// BindFlags populates cmd.Flags() based on the exported fields of cfg (must be pointer to struct).
// It looks for a `flag:"..."` tag to name each flag; otherwise it kebab-cases the Go field name.
// Usage: in your main() or init(), call BindFlags(rootCmd, &cfg) *before* executing the command.
func BindFlagSet(prefixPath string, cfgList ...interface{}) (*pflag.FlagSet, error) {
	var flagset *pflag.FlagSet
	for _, cfg := range cfgList {
		fs, err := BindFlags(cfg, prefixPath)
		if err != nil {
			return nil, err
		}
		if flagset == nil {
			flagset = fs
		} else {
			flagset.AddFlagSet(fs)
		}
	}
	return flagset, nil

}
func BindFlags(cfg interface{}, prefixPath string) (*pflag.FlagSet, error) {
	rv := reflect.ValueOf(cfg)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return nil, errors.New("cfg must be a pointer to a struct")
	}
	rs := rv.Elem()
	rt := rs.Type()

	fs := pflag.NewFlagSet(rt.Name(), pflag.ExitOnError)
	prefix := ToKebabCase(prefixPath + rt.Name())
	re := regexp.MustCompile("([a-z0-9])([A-Z])")

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rs.Field(i)
		if !fv.CanSet() {
			continue
		}

		name := field.Tag.Get("flag")
		if name == "" {
			name = re.ReplaceAllString(field.Name, "${1}-${2}")
			name = strings.ToLower(name)
		}
		usage := field.Tag.Get("usage")
		name = prefix + "-" + name
		// (optional) set Viper’s default from the struct too
		viper.SetDefault(name, fv.Interface())

		switch fv.Kind() {
		case reflect.String:
			ptr := fv.Addr().Interface().(*string)
			fs.StringVar(ptr, name, fv.String(), usage)

		case reflect.Bool:
			ptr := fv.Addr().Interface().(*bool)
			fs.BoolVar(ptr, name, fv.Bool(), usage)

		case reflect.Int:
			ptr := fv.Addr().Interface().(*int)
			fs.IntVar(ptr, name, int(fv.Int()), usage)
		case reflect.Int8:
			ptr := fv.Addr().Interface().(*int8)
			fs.Int8Var(ptr, name, int8(fv.Int()), usage)
		case reflect.Int16:
			ptr := fv.Addr().Interface().(*int16)
			fs.Int16Var(ptr, name, int16(fv.Int()), usage)
		case reflect.Int32:
			ptr := fv.Addr().Interface().(*int32)
			fs.Int32Var(ptr, name, int32(fv.Int()), usage)
		case reflect.Int64:
			ptr := fv.Addr().Interface().(*int64)
			fs.Int64Var(ptr, name, fv.Int(), usage)

		case reflect.Uint:
			ptr := fv.Addr().Interface().(*uint)
			fs.UintVar(ptr, name, uint(fv.Uint()), usage)
		case reflect.Uint8:
			ptr := fv.Addr().Interface().(*uint8)
			fs.Uint8Var(ptr, name, uint8(fv.Uint()), usage)
		case reflect.Uint16:
			ptr := fv.Addr().Interface().(*uint16)
			fs.Uint16Var(ptr, name, uint16(fv.Uint()), usage)
		case reflect.Uint32:
			ptr := fv.Addr().Interface().(*uint32)
			fs.Uint32Var(ptr, name, uint32(fv.Uint()), usage)
		case reflect.Uint64:
			ptr := fv.Addr().Interface().(*uint64)
			fs.Uint64Var(ptr, name, fv.Uint(), usage)

		case reflect.Float32:
			ptr := fv.Addr().Interface().(*float32)
			fs.Float32Var(ptr, name, float32(fv.Float()), usage)
		case reflect.Float64:
			ptr := fv.Addr().Interface().(*float64)
			fs.Float64Var(ptr, name, fv.Float(), usage)

		case reflect.Slice:
			if fv.Type().Elem().Kind() == reflect.String {
				ptr := fv.Addr().Interface().(*[]string)
				fs.StringSliceVar(ptr, name, fv.Interface().([]string), usage)
			}
		default:
			continue
		}

		// re-bind into Viper & ENV
		if err := viper.BindPFlag(name, fs.Lookup(name)); err != nil {
			return nil, fmt.Errorf("bind flag %q: %w", name, err)
		}
		if err := viper.BindEnv(name); err != nil {
			return nil, fmt.Errorf("bind env %q: %w", name, err)
		}
	}

	return fs, nil
}

// LoadConfig reads all flags from cmd.Flags() (and any bound ENV vars)
// into a new zero‐value T (which must be a struct type), and returns it.

func LoadConfig[T any](cmd *cobra.Command, prefixPath string) (T, error) {
	var cfg T

	// 0) T must be a struct
	rtT := reflect.TypeOf(cfg)
	if rtT.Kind() != reflect.Struct {
		return cfg, fmt.Errorf("LoadConfig[T]: T must be a struct, got %s", rtT.Kind())
	}

	// 1) Addressable Value for &cfg
	rv := reflect.ValueOf(&cfg).Elem() // struct
	rt := rv.Type()

	// 2) Compute kebab‐prefix
	prefix := ToKebabCase(prefixPath + rt.Name())
	re := regexp.MustCompile("([a-z0-9])([A-Z])")

	// 3) Walk fields
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fv := rv.Field(i)
		if !fv.CanSet() {
			continue
		}

		// 3a) Build flag name: tag or camel→kebab
		name := field.Tag.Get("flag")
		if name == "" {
			name = re.ReplaceAllString(field.Name, "${1}-${2}")
			name = strings.ToLower(name)
		}
		if prefix != "" {
			name = prefix + "-" + name
		}

		// 3b) Build env var key: PREFIX_FIELD_NAME
		envKey := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))

		var (
			parseErr error
			setFrom  string // "env" or "flag"
		)

		// 3c) helper to parse & assign
		assign := func(val interface{}) {
			v := reflect.ValueOf(val)
			// if val is a pointer (e.g. *string), Elem(); else direct
			if v.Kind() == reflect.Ptr {
				v = v.Elem()
			}
			fv.Set(v.Convert(fv.Type()))
		}

		// 4) Try ENV first
		if ev := os.Getenv(envKey); ev != "" {
			setFrom = "env"
			switch fv.Kind() {
			case reflect.String:
				assign(ev)

			case reflect.Bool:
				var b bool
				b, parseErr = strconv.ParseBool(ev)
				assign(b)

			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				var si int64
				si, parseErr = strconv.ParseInt(ev, 10, fv.Type().Bits())
				assign(si)

			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				var ui uint64
				ui, parseErr = strconv.ParseUint(ev, 10, fv.Type().Bits())
				assign(ui)

			case reflect.Float32, reflect.Float64:
				var f float64
				f, parseErr = strconv.ParseFloat(ev, fv.Type().Bits())
				assign(f)

			case reflect.Slice:
				if fv.Type().Elem().Kind() == reflect.String {
					// comma-split
					parts := strings.Split(ev, ",")
					assign(parts)
				}
			default:
				// unsupported: skip
				continue
			}

		} else {
			// 5) Fallback to flag
			setFrom = "flag"
			switch fv.Kind() {
			case reflect.String:
				var v string
				v, parseErr = cmd.Flags().GetString(name)
				assign(v)

			case reflect.Bool:
				var v bool
				v, parseErr = cmd.Flags().GetBool(name)
				assign(v)

			case reflect.Int:
				var v int
				v, parseErr = cmd.Flags().GetInt(name)
				assign(v)
			case reflect.Int8:
				var v int
				v, parseErr = cmd.Flags().GetInt(name)
				assign(int8(v))
			case reflect.Int16:
				var v int
				v, parseErr = cmd.Flags().GetInt(name)
				assign(int16(v))
			case reflect.Int32:
				var v32 int32
				v32, parseErr = cmd.Flags().GetInt32(name)
				assign(v32)
			case reflect.Int64:
				var v64 int64
				v64, parseErr = cmd.Flags().GetInt64(name)
				assign(v64)

			case reflect.Uint:
				var u uint
				u, parseErr = cmd.Flags().GetUint(name)
				assign(u)
			case reflect.Uint8:
				var u uint
				u, parseErr = cmd.Flags().GetUint(name)
				assign(uint8(u))
			case reflect.Uint16:
				var u uint
				u, parseErr = cmd.Flags().GetUint(name)
				assign(uint16(u))
			case reflect.Uint32:
				var u32 uint32
				u32, parseErr = cmd.Flags().GetUint32(name)
				assign(u32)
			case reflect.Uint64:
				var u64 uint64
				u64, parseErr = cmd.Flags().GetUint64(name)
				assign(u64)

			case reflect.Float32:
				var f32 float32
				f32, parseErr = cmd.Flags().GetFloat32(name)
				assign(f32)
			case reflect.Float64:
				var f64 float64
				f64, parseErr = cmd.Flags().GetFloat64(name)
				assign(f64)

			case reflect.Slice:
				if fv.Type().Elem().Kind() == reflect.String {
					var ss []string
					ss, parseErr = cmd.Flags().GetStringSlice(name)
					assign(ss)
				}
			default:
				continue
			}
		}

		// 6) Any parse error?
		if parseErr != nil {
			return cfg, fmt.Errorf("failed to parse %s value for %q: %w", setFrom, name, parseErr)
		}
	}

	return cfg, nil
}

var (
	// match wherever a lower- or digit-char is followed by an upper-char
	matchLowerToUpper = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	// match wherever a sequence of upper-chars is followed by a lower-char,
	// so we split acronyms like “HTTPServer” → “HTTP-Server”
	matchAcronymToWord = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)
)

// ToKebabCase converts "PascalCase" or "camelCase" to "kebab-case"
func ToKebabCase(s string) string {
	// first, split acronyms: e.g. "HTTPServer" → "HTTP-Server"
	s = matchAcronymToWord.ReplaceAllString(s, "${1}-${2}")
	// next, split any lower-to-upper boundary: "pascalCase" → "pascal-Case"
	s = matchLowerToUpper.ReplaceAllString(s, "${1}-${2}")
	// finally lowercase everything
	return strings.ToLower(s)
}

func PrintAllFlags(fs *pflag.FlagSet) {
	fs.VisitAll(func(f *pflag.Flag) {
		// f.Name is the long form (e.g. "foo")
		// f.Shorthand is the short form (e.g. "f"), empty if none
		// f.Value.Type() is the string representation of the underlying type
		// f.DefValue is the default value, as a string
		// f.Usage is the help string

		fmt.Printf("%s: %v \n",
			strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_")), f.Value,
		)

	})
}
