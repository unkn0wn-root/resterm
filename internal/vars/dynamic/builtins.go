package dynamic

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultStringLen = 16
	maxStringLen     = 4096
	// randomIntMax is the range the argument-free form produced before
	// helpers took arguments.
	randomIntMax = 1 << 62
)

// builtins is the complete set of {{$...}} helpers, in documentation order.
// A new helper is one entry here. eval must stay cheap and free of side
// effects, since Validate runs it to check a reference at compile time.
var builtins = []helper{
	{
		name:    "$uuid",
		aliases: []string{"$guid"},
		summary: "Random UUID v4",
		eval:    evalUUID,
	},
	{
		name:    "$timestamp",
		summary: "Unix time in seconds",
		offset:  true,
		eval:    evalTimestamp,
	},
	{
		name:    "$timestampMs",
		summary: "Unix time in milliseconds",
		offset:  true,
		eval:    evalTimestampMs,
	},
	{
		name:    "$timestampISO8601",
		summary: "Current time, RFC3339 UTC",
		offset:  true,
		eval:    evalTimestampISO,
	},
	{
		name:    "$randomInt",
		summary: "Random integer, within min and max when given",
		usage:   "$randomInt(1, 100)",
		args:    arity{max: 2},
		eval:    evalRandomInt,
	},
	{
		name:    "$randomString",
		summary: "Random alphanumeric string, 16 characters by default",
		usage:   "$randomString(24)",
		args:    arity{max: 1},
		eval:    evalRandomString,
	},
	{
		name:    "$randomChoice",
		summary: "Random value from the given list",
		usage:   `$randomChoice("a", "b", "c")`,
		args:    arity{min: 1, max: -1},
		eval:    evalRandomChoice,
	},
	{
		name:    "$randomName",
		summary: "Random full name",
		eval:    gen(fakePerson),
	},
	{
		name:    "$randomEmail",
		summary: "Random email address on an example domain",
		eval:    gen(fakeEmail),
	},
	{
		name:    "$fake.person",
		summary: "Random full name",
		eval:    gen(fakePerson),
	},
	{
		name:    "$fake.firstName",
		summary: "Random first name",
		eval:    gen(fakeFirstName),
	},
	{
		name:    "$fake.lastName",
		summary: "Random last name",
		eval:    gen(fakeLastName),
	},
	{
		name:    "$fake.email",
		summary: "Random email address on an example domain",
		eval:    gen(fakeEmail),
	},
	{
		name:    "$fake.username",
		summary: "Random username",
		eval:    gen(fakeUsername),
	},
	{
		name:    "$fake.company",
		summary: "Random company name",
		eval:    gen(fakeCompany),
	},
	{
		name:    "$fake.domain",
		summary: "Random hostname under example.com",
		eval:    gen(fakeDomain),
	},
	{
		name:    "$fake.city",
		summary: "Random city",
		eval:    gen(fakeCity),
	},
	{
		name:    "$fake.country",
		summary: "Random country",
		eval:    gen(fakeCountry),
	},
	{
		name:    "$fake.phone",
		summary: "Random phone number from a range reserved for fiction",
		eval:    gen(fakePhone),
	},
	{
		name:    "$fake.word",
		summary: "Random single word",
		eval:    gen(fakeWord),
	},
	{
		name:    "$fake.sentence",
		summary: "Random short sentence",
		eval:    gen(fakeSentence),
	},
}

var index = buildIndex()

func buildIndex() map[string]helper {
	m := make(map[string]helper, 2*len(builtins))
	for _, h := range builtins {
		add := func(name string) {
			key := strings.ToLower(name)
			if _, dup := m[key]; dup {
				panic("dynamic: duplicate helper name " + name)
			}
			m[key] = h
		}
		add(h.name)
		for _, alias := range h.aliases {
			add(alias)
		}
	}
	return m
}

func lookup(name string) (helper, bool) {
	h, ok := index[strings.ToLower(strings.TrimSpace(name))]
	return h, ok
}

// gen adapts a plain generator to a helper that takes no arguments.
func gen(fn func() string) func(call) (string, error) {
	return func(call) (string, error) { return fn(), nil }
}

func evalUUID(call) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("$uuid: %w", err)
	}
	return id.String(), nil
}

func evalTimestamp(c call) (string, error) {
	return strconv.FormatInt(time.Now().Add(c.offset).Unix(), 10), nil
}

func evalTimestampMs(c call) (string, error) {
	return strconv.FormatInt(time.Now().Add(c.offset).UnixMilli(), 10), nil
}

func evalTimestampISO(c call) (string, error) {
	return time.Now().Add(c.offset).UTC().Format(time.RFC3339), nil
}

func evalRandomInt(c call) (string, error) {
	switch len(c.args) {
	case 0:
		return strconv.FormatUint(randUint64(randomIntMax), 10), nil
	case 1:
		hi, err := c.argInt(0)
		if err != nil {
			return "", err
		}
		return randomIntRange(c, 0, hi)
	default:
		lo, err := c.argInt(0)
		if err != nil {
			return "", err
		}
		hi, err := c.argInt(1)
		if err != nil {
			return "", err
		}
		return randomIntRange(c, lo, hi)
	}
}

// randomIntRange picks from lo to hi inclusive.
func randomIntRange(c call, lo, hi int64) (string, error) {
	if lo > hi {
		return "", fmt.Errorf("%s: minimum %d is above maximum %d", c.helper.name, lo, hi)
	}
	// Unsigned arithmetic keeps the inclusive span exact even across the full
	// int64 range, where it wraps to zero and every value is allowed.
	span := uint64(hi) - uint64(lo) + 1
	if span == 0 {
		return strconv.FormatInt(int64(randUint64Full()), 10), nil
	}
	return strconv.FormatInt(lo+int64(randUint64(span)), 10), nil
}

func evalRandomString(c call) (string, error) {
	n := int64(defaultStringLen)
	if len(c.args) == 1 {
		size, err := c.argInt(0)
		if err != nil {
			return "", err
		}
		n = size
	}
	if n < 1 || n > maxStringLen {
		return "", fmt.Errorf("%s: length must be between 1 and %d, got %d", c.helper.name, maxStringLen, n)
	}
	return randomString(int(n)), nil
}

func evalRandomChoice(c call) (string, error) {
	return pick(c.args), nil
}
