package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/dgryski/go-wyhash"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// spices is a list of common spices
var spices = []string{
	"allspice", "anise", "basil", "bay", "black pepper", "cardamom", "cayenne",
	"cinnamon", "cloves", "coriander", "cumin", "curry", "dill", "fennel", "fenugreek",
	"garlic", "ginger", "marjoram", "mustard", "nutmeg", "oregano", "paprika", "parsley",
	"pepper", "rosemary", "saffron", "sage", "salt", "tarragon", "thyme", "turmeric", "vanilla",
	"caraway", "chili", "masala", "lemongrass", "mint", "poppy", "sesame", "sumac", "mace",
	"nigella", "peppercorn", "wasabi",
}

// adjectives is a list of common adjectives
var adjectives = []string{
	"able", "bad", "best", "better", "big", "black", "certain", "clear", "different", "early",
	"easy", "economic", "federal", "free", "full", "good", "great", "hard", "high", "human",
	"important", "international", "large", "late", "little", "local", "long", "low", "major",
	"military", "national", "new", "old", "only", "other", "political", "possible", "public",
	"real", "recent", "right", "small", "social", "special", "strong", "sure", "true", "white",
	"whole", "young",
}

// nouns is a list of common nouns
var nouns = []string{
	"angle", "ant", "apple", "arch", "arm", "army", "baby", "bag", "ball", "band", "basin", "basket", "bath", "bed", "bee", "bell",
	"berry", "bird", "blade", "board", "boat", "bone", "book", "boot", "bottle", "box", "boy", "brain", "brake", "branch", "brick", "bridge",
	"brush", "bucket", "bulb", "button", "cake", "camera", "card", "carriage", "cart", "cat", "chain", "cheese", "chess", "chin", "church", "circle",
	"clock", "cloud", "coat", "collar", "comb", "cord", "cow", "cup", "curtain", "cushion", "dog", "door", "drain", "drawer", "dress", "drop",
	"ear", "egg", "engine", "eye", "face", "farm", "feather", "finger", "fish", "flag", "floor", "fly", "foot", "fork", "fowl", "frame",
	"garden", "girl", "glove", "goat", "gun", "hair", "hammer", "hand", "hat", "head", "heart", "hook", "horn", "horse", "hospital", "house",
	"island", "jewel", "kettle", "key", "knee", "knife", "knot", "leaf", "leg", "library", "line", "lip", "lock", "map", "match", "monkey",
	"moon", "mouth", "muscle", "nail", "neck", "needle", "nerve", "net", "nose", "nut", "office", "orange", "oven", "parcel", "pen", "pencil",
	"picture", "pig", "pin", "pipe", "plane", "plate", "plough", "pocket", "pot", "potato", "prison", "pump", "rail", "rat", "receipt", "ring",
	"rod", "roof", "root", "sail", "school", "scissors", "screw", "seed", "sheep", "shelf", "ship", "shirt", "shoe", "skin", "skirt", "snake",
	"sock", "spade", "sponge", "spoon", "spring", "square", "stamp", "star", "station", "stem", "stick", "stocking", "stomach", "store", "street", "sun",
	"table", "tail", "thread", "throat", "thumb", "ticket", "toe", "tongue", "tooth", "town", "train", "tray", "tree", "trousers", "umbrella", "wall",
	"watch", "wheel", "whip", "whistle", "window", "wing", "wire", "worm",
}

type Rng struct {
	rng *rand.Rand
}

func NewRng(s string) Rng {
	return Rng{rand.New(rand.NewSource(int64(wyhash.Hash([]byte(s), 2467825690))))}
}

func (r Rng) Intn(n int) int64 {
	return int64(r.rng.Intn(n))
}

// Chooses a random element from a slice of strings.
func (r Rng) Choice(a []string) string {
	return a[r.Intn(len(a))]
}

// Chooses a random element from a slice of strings, with a quadratic bias
// towards the first elements.
func (r Rng) QuadraticChoice(a []string) string {
	sq := float64(len(a) * len(a))
	rn := r.Float(0, sq)
	choice := len(a) - int(math.Floor(math.Sqrt(rn))) - 1
	return a[choice]
}

func (r Rng) Bool() bool {
	return r.Intn(2) == 0
}

func (r Rng) Int(min, max int) int64 {
	return int64(r.rng.Intn(max-min) + min)
}

func (r Rng) Float(min, max float64) float64 {
	return r.rng.Float64()*(max-min) + min
}

func (r Rng) Gaussian(mean, stddev float64) float64 {
	return r.rng.NormFloat64()*stddev + mean
}

func (r Rng) GaussianInt(mean, stddev float64) int64 {
	return int64(r.rng.NormFloat64()*stddev + mean)
}

func (r Rng) String(len int) string {
	var b strings.Builder
	for i := 0; i < len; i++ {
		b.WriteByte(byte("abcdefghijklmnopqrstuvwxyz"[r.Int(0, 26)]))
	}
	return b.String()
}

func (r Rng) HexString(len int) string {
	var b strings.Builder
	for i := 0; i < len; i++ {
		b.WriteByte(byte("0123456789abcdef"[r.Int(0, 16)]))
	}
	return b.String()
}

func (r Rng) WordPair() string {
	return r.Choice(adjectives) + "-" + r.Choice(nouns)
}

func (r Rng) BoolWithProb(p int) bool {
	return r.Int(0, 100) < int64(p)
}

func getGoroutineID() uint64 {
	var buffer [31]byte
	written := runtime.Stack(buffer[:], false)
	index := 10
	negative := buffer[index] == '-'
	if negative {
		index = 11
	}
	id := uint64(0)
	for index < written {
		byte := buffer[index]
		if byte == ' ' {
			break
		}
		if byte < '0' || byte > '9' {
			panic("could not get goroutine ID")
		}
		id *= 10
		id += uint64(byte - '0')
		index++
	}
	if negative {
		id = -id
	}
	return id
}

// getProcessID returns the process ID
func getProcessID() int64 {
	return int64(os.Getpid())
}

func (r Rng) getValueGenerators() []func() any {
	return []func() any{
		func() any { return r.Intn(100) },
		func() any { return r.BoolWithProb(99) },
		func() any { return r.BoolWithProb(50) },
		func() any { return r.BoolWithProb(1) },
		func() any { return r.Int(-100, 100) },
		func() any { return r.Float(0, 1000) },
		func() any { return r.Float(0, 1) },
		func() any { return r.GaussianInt(50, 30) },
		func() any { return r.Gaussian(10000, 1000) },
		func() any { return r.Gaussian(500, 300) },
		func() any { return r.String(2) },
		func() any { return r.String(5) },
		func() any { return r.String(10) },
		func() any { return r.String(4) + "-" + r.HexString(8) + "-" + r.String(4) },
		func() any { return r.HexString(16) },
	}
}

// getWordList returns a list of words with the specified cardinality;
// if a source word list is specified and cardinality fits within it, it uses it.
func getWordList(rng Rng, cardinality int, source []string) []string {
	generator := rng.WordPair
	if source != nil && len(source) >= cardinality {
		generator = func() string { return rng.Choice(source) }
	}
	words := make([]string, cardinality)
	for i := 0; i < cardinality; i++ {
		words[i] = generator()
	}
	return words
}

// parseUserFields expects a list of fields in the form of name=constant or name=/gen.
// See README.md for more information.
func parseUserFields(rng Rng, userfields []string) (map[string]func() any, error) {
	constpat := regexp.MustCompile(`^([a-zA-Z0-9_]+)=([^/].*)$`)
	genpat := regexp.MustCompile(`^((?:[0-9]+\.)?[a-zA-Z0-9_]+)=/([ibfsu][awxrgqt]?)([0-9.-]+)?(,[0-9.-]+)?$`)
	// groups                                        1                   2	         3         4
	fields := make(map[string]func() any)
	for _, field := range userfields {
		// see if it's a constant
		matches := constpat.FindStringSubmatch(field)
		if matches != nil {
			name := matches[1]
			value := matches[2]
			fields[name] = getConst(value)
			continue
		}

		// see if it's a generator
		matches = genpat.FindStringSubmatch(field)
		if matches == nil {
			return nil, fmt.Errorf("unparseable user field %s", field)
		}
		var err error
		name := matches[1]
		gentype := matches[2]
		p1 := matches[3]
		p2 := matches[4]
		switch gentype {
		case "i", "ir", "ig":
			fields[name], err = getIntGen(rng, gentype, p1, p2)
			if err != nil {
				return nil, fmt.Errorf("invalid int in user field %s: %w", field, err)
			}
		case "f", "fr", "fg":
			fields[name], err = getFloatGen(rng, gentype, p1, p2)
			if err != nil {
				return nil, fmt.Errorf("invalid float in user field %s: %w", field, err)
			}
		case "b":
			n := 50
			var err error
			if p1 != "" {
				n, err = strconv.Atoi(p1)
				if err != nil || n < 0 || n > 100 {
					return nil, fmt.Errorf("invalid bool option in %s", field)
				}
			}
			fields[name] = func() any { return rng.BoolWithProb(n) }
		case "s", "sw", "sx", "sa", "sq":
			n := 16
			if p1 != "" {
				n, err = strconv.Atoi(p1)
				if err != nil {
					return nil, fmt.Errorf("invalid string option in %s", field)
				}
			}
			switch gentype {
			case "sw":
				// words with specified cardinality in a rectangular distribution
				words := getWordList(rng, n, nil)
				fields[name] = func() any { return rng.Choice(words) }
			case "sq":
				// words with specified cardinality in a quadratic distribution
				words := getWordList(rng, n, nil)
				fields[name] = func() any { return rng.QuadraticChoice(words) }
			case "sx":
				fields[name] = func() any { return rng.HexString(n) }
			default:
				fields[name] = func() any { return rng.String(n) }
			}
		case "u", "uq":
			// Generate a URL-like string with a random path and possibly a query string
			fields[name], err = getURLGen(rng, gentype, p1, p2)
			if err != nil {
				return nil, fmt.Errorf("invalid float in user field %s: %w", field, err)
			}
		case "st":
			// Generate a semi-plausible mix of status codes; percentage of 400s and 500s can be controlled by the extra args
			twos := 95.0
			fours := 4.0
			fives := 1.0
			if p1 != "" {
				fours, err = strconv.ParseFloat(p1, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid float in user field %s: %w", field, err)
				}
			}
			if p2 != "" {
				fives, err = strconv.ParseFloat(p2[1:], 64)
				if err != nil {
					return nil, fmt.Errorf("invalid float in user field %s: %w", field, err)
				}
			}
			twos = 100 - fours - fives
			fields[name] = func() any {
				r := rng.Float(0, 100)
				if r < twos {
					return rng.QuadraticChoice([]string{"200", "200", "200", "201", "202"})
				} else if r < twos+fours {
					return rng.QuadraticChoice([]string{"404", "400", "400", "400", "402", "429", "403"})
				} else {
					return "500"
				}
			}

		default:
			return nil, fmt.Errorf("invalid generator type %s in field %s", gentype, field)
		}
	}
	return fields, nil
}

func getConst(value string) func() any {
	var gen func() any
	if value == "true" {
		gen = func() any { return true }
	} else if value == "false" {
		gen = func() any { return false }
	} else {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			gen = func() any { return i }
		} else if f, err := strconv.ParseFloat(value, 64); err == nil {
			gen = func() any { return f }
		} else {
			gen = func() any { return value }
		}
	}
	return gen
}

func gaussianDefaults(v1, v2 float64) (float64, float64) {
	if v1 == 0 && v2 == 0 {
		v1 = 100
		v2 = 10
	} else if v2 == 0 {
		v2 = v1 / 10
	}
	return v1, v2
}

func getIntGen(rng Rng, gentype, p1, p2 string) (func() any, error) {
	var v1, v2 int
	var err error
	if p1 == "" {
		v1 = 0
	} else {
		v1, err = strconv.Atoi(p1)
		if err != nil {
			return nil, fmt.Errorf("%s is not an int", p1)
		}
	}
	if p2 == "" || p2 == "," {
		v2 = v1
		v1 = 0
	} else {
		v2, err = strconv.Atoi(p2[1:])
		if err != nil {
			return nil, fmt.Errorf("%s is not an int", p2[:1])
		}
	}
	if gentype == "ig" {
		g1, g2 := gaussianDefaults(float64(v1), float64(v2))
		return func() any { return rng.GaussianInt(g1, g2) }, nil
	} else {
		if v1 == 0 && v2 == 0 {
			v2 = 100
		}
		return func() any { return rng.Int(v1, v2) }, nil
	}
}

func getFloatGen(rng Rng, gentype, p1, p2 string) (func() any, error) {
	var v1, v2 float64
	var err error
	if p1 == "" {
		v1 = 0
	} else {
		v1, err = strconv.ParseFloat(p1, 64)
		if err != nil {
			return nil, fmt.Errorf("%s is not a number", p1)
		}
	}
	if p2 == "" || p2 == "," {
		v2 = v1
		v1 = 0
	} else {
		v2, err = strconv.ParseFloat(p2[1:], 64)
		if err != nil {
			return nil, fmt.Errorf("%s is not a number", p2[:1])
		}
	}
	if gentype == "fg" {
		g1, g2 := gaussianDefaults(v1, v2)
		return func() any { return rng.GaussianInt(g1, g2) }, nil
	} else {
		if v1 == 0 && v2 == 0 {
			v2 = 100
		}
		return func() any { return rng.Float(v1, v2) }, nil
	}
}

func getURLGen(rng Rng, gentype, p1, p2 string) (func() any, error) {
	var c1 int = 3
	var c2 int = 10
	var err error
	if p1 != "" {
		c1, err = strconv.Atoi(p1)
		if err != nil {
			return nil, fmt.Errorf("%s is not a number", p1)
		}
	}
	if p2 != "" && p2 != "," {
		c2, err = strconv.Atoi(p2[1:])
		if err != nil {
			return nil, fmt.Errorf("%s is not a number", p2[:1])
		}
	}
	path1words := getWordList(rng, c1, nouns)
	path1 := func() string { return rng.Choice(path1words) }
	path2 := func() string { return "" }
	if c2 != 0 {
		path2words := getWordList(rng, c2, adjectives)
		path2 = func() string { return rng.Choice(path2words) }
	}
	if gentype == "uq" {
		return func() any {
			return "https://example.com/" + path1() + "/" + path2() + "?extra=" + rng.String(10)
		}, nil
	} else {
		return func() any {
			return "https://example.com/" + path1() + "/" + path2()
		}, nil
	}
}

type Fielder struct {
	fields map[string]func() any
	names  []string
}

// Fielder is an object that takes a name and generates a map of
// fields based on using the name as a random seed.
// It takes a set of field specifications that are used to generate the fields.
// It also takes two counts: the number of fields to generate and the number of
// service names to generate. The field names are randomly generated by
// combining an adjective and a noun and are consistent for a given fielder.
// The field values are randomly generated.
// Fielder also includes two special fields: goroutine_id and process_id.
func NewFielder(seed string, userFields []string, nextras, nservices int) (*Fielder, error) {
	rng := NewRng(seed)
	gens := rng.getValueGenerators()
	fields, err := parseUserFields(rng, userFields)
	if err != nil {
		return nil, err
	}
	for i := 0; i < nextras; i++ {
		fieldname := rng.WordPair()
		fields[fieldname] = gens[rng.Intn(len(gens))]
	}
	fields["goroutine_id"] = func() any { return getGoroutineID() }
	fields["process_id"] = func() any { return getProcessID() }

	names := make([]string, nservices)
	for i := 0; i < nservices; i++ {
		names[i] = rng.Choice(spices)
	}
	return &Fielder{fields: fields, names: names}, nil
}

func (f *Fielder) GetServiceName(n int) string {
	return f.names[n%len(f.names)]
}

// Searches for a field name that includes a level marker.
// These markers look like "1.fieldname" and are used to
// indicate that the field should be included at a specific
// level in the trace, where 0 is the root.
func (f *Fielder) atLevel(name string, level int) (string, bool) {
	keypat := regexp.MustCompile(`^([0-9]+)\.(.*$)`)
	matches := keypat.FindStringSubmatch(name)
	if len(matches) == 0 {
		return name, true
	}
	keylevel, _ := strconv.Atoi(matches[1])
	if keylevel == level {
		return matches[2], true
	}
	return matches[2], false
}

func (f *Fielder) GetFields(count int64, level int) map[string]any {
	fields := make(map[string]any)
	if count != 0 {
		fields["count"] = count
	}
	for k, v := range f.fields {
		k, ok := f.atLevel(k, level)
		if !ok {
			continue
		}
		fields[k] = v()
	}
	return fields
}

func (f *Fielder) AddFields(span trace.Span, count int64, level int) {
	if count != 0 {
		span.SetAttributes(attribute.Int64("count", count))
	}
	for key, val := range f.fields {
		key, ok := f.atLevel(key, level)
		if !ok {
			continue
		}
		switch v := val().(type) {
		case int64:
			span.SetAttributes(attribute.Int64(key, v))
		case uint64:
			span.SetAttributes(attribute.Int64(key, int64(v)))
		case float64:
			span.SetAttributes(attribute.Float64(key, v))
		case string:
			span.SetAttributes(attribute.String(key, v))
		case bool:
			span.SetAttributes(attribute.Bool(key, v))
		default:
			panic(fmt.Sprintf("unknown type %T for %s -- implementation error in fielder.go", v, key))
		}
	}
}
