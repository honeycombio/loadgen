package main

import (
	"fmt"
	"math/rand"
	"os"
	"runtime"

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

// adjectives is a list of 30 common adjectives
var adjectives = []string{
	"able", "bad", "best", "better", "big", "black", "certain", "clear", "different", "early",
	"easy", "economic", "federal", "free", "full", "good", "great", "hard", "high", "human",
	"important", "international", "large", "late", "little", "local", "long", "low", "major",
	"military", "national", "new", "old", "only", "other", "political", "possible", "public",
	"real", "recent", "right", "small", "social", "special", "strong", "sure", "true", "white",
	"whole", "young",
}

// nouns is a list of 30 common nouns
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

func (r Rng) Choice(a []string) string {
	return a[r.Intn(len(a))]
}

func (r Rng) Bool() bool {
	return r.Intn(2) == 0
}

func (r Rng) Int(min, max int) int64 {
	return int64(r.rng.Intn((max - min) + min))
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

// #   - int (rectangular min/max)
// #   - int (gaussian mean/stddev)
// #   - int upcounter
// #   - int updowncounter (min/max)
// #   - float (rectangular min/max)
// #   - float (gaussian mean/stddev)
// #   - string (from list)
// #   - string (random min/max length)
// #   - bool

func (r Rng) getRandomGenerators() []func() any {
	return []func() any{
		func() any { return r.Intn(100) },
		func() any { return r.Bool() },
		func() any { return r.Int(-100, 100) },
		func() any { return r.Float(0, 1000) },
		func() any { return r.Float(0, 1) },
		func() any { return r.GaussianInt(50, 30) },
		func() any { return r.Gaussian(10000, 1000) },
		func() any { return r.Gaussian(500, 300) },
	}
}

type Fielder struct {
	fields map[string]func() any
	names  []string
}

// Fielder is an object that takes a name and a count and generates a map of
// random fields based on using the name as a random seed. It takes two counts:
// the number of fields to generate and the number of service names to generate. The field names are randomly generated by
// combining an adjective and a noun and are consistent for a given fielder.
// The field values are randomly generated.
// Fielder also includes two special fields: goroutine_id and process_id.
func NewFielder(name string, nfields, nservices int) *Fielder {
	fields := make(map[string]func() any)
	rng := NewRng(name)
	gens := rng.getRandomGenerators()
	for i := 0; i < nfields; i++ {
		fieldname := rng.Choice(adjectives) + "-" + rng.Choice(nouns)
		fields[fieldname] = gens[rng.Intn(len(gens))]
	}
	fields["goroutine_id"] = func() any { return getGoroutineID() }
	fields["process_id"] = func() any { return getProcessID() }

	names := make([]string, nservices)
	for i := 0; i < nservices; i++ {
		names[i] = rng.Choice(spices)
	}
	return &Fielder{fields: fields, names: names}
}

func (f *Fielder) GetServiceName(n int) string {
	return f.names[n%len(f.names)]
}

func (f *Fielder) GetFields(count int64) map[string]any {
	fields := make(map[string]any)
	if count != 0 {
		fields["count"] = count
	}
	for k, v := range f.fields {
		fields[k] = v()
	}
	return fields
}

func (f *Fielder) AddFields(span trace.Span, count int64) {
	if count != 0 {
		span.SetAttributes(attribute.Int64("count", count))
	}
	for key, val := range f.fields {
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
