package dynamic

import "strings"

// Sample data for the $fake and $random helpers. Pools are plain ASCII so
// generated names stay usable inside emails and hostnames. Request files are
// executed, so helper output must never point a request at a real host or
// mailbox: addresses and hosts stay under the RFC 2606 example domains, and
// phone numbers inside the fictional 555-01xx block.

var (
	firstNames = []string{
		"Ada", "Amara", "Anders", "Bianca", "Camille", "Dario", "Elena", "Farid",
		"Grace", "Hugo", "Ines", "Jonas", "Keiko", "Lucas", "Maya", "Nadia",
		"Omar", "Priya", "Quentin", "Rafael", "Sofia", "Tomas", "Uma", "Viktor",
		"Wren", "Yusuf", "Zara",
	}
	lastNames = []string{
		"Alvarez", "Bennett", "Castillo", "Dubois", "Eriksen", "Fontaine",
		"Gallagher", "Haddad", "Ibarra", "Jensen", "Kowalski", "Laurent",
		"Moreau", "Novak", "Okafor", "Petrov", "Quinn", "Ramirez", "Sorensen",
		"Tanaka", "Vargas", "Whitfield", "Yilmaz", "Zhang",
	}
	companyNames = []string{
		"Anchor", "Beacon", "Brightpath", "Cascade", "Cobalt", "Fieldstone",
		"Harbor", "Ironwood", "Lakeside", "Meridian", "Northwind", "Pioneer",
		"Redwood", "Silverline", "Summit", "Trailhead",
	}
	companySuffixes = []string{
		"Analytics", "Foods", "Holdings", "Industries", "Labs", "Logistics",
		"Media", "Partners", "Robotics", "Supply", "Systems", "Works",
	}
	cities = []string{
		"Aarhus", "Bergen", "Cordoba", "Dresden", "Fukuoka", "Galway",
		"Helsinki", "Innsbruck", "Jaipur", "Krakow", "Lucerne", "Maribor",
		"Nantes", "Ostrava", "Porto", "Salerno", "Tampere", "Utrecht", "Valencia",
	}
	countries = []string{
		"Argentina", "Belgium", "Canada", "Denmark", "Estonia", "Finland",
		"Hungary", "Iceland", "Japan", "Kenya", "Latvia", "Malta", "Norway",
		"Oman", "Peru", "Romania", "Slovenia", "Thailand", "Uruguay", "Vietnam",
	}
	words = []string{
		"amber", "brisk", "cedar", "delta", "ember", "fjord", "glint", "harbor",
		"ivory", "jetty", "kelp", "lumen", "marsh", "nimbus", "onyx", "pebble",
		"quartz", "ripple", "spruce", "tide", "umber", "vellum", "willow", "zenith",
	}
	emailDomains = []string{"example.com", "example.net", "example.org"}
)

func fakeFirstName() string {
	return pick(firstNames)
}

func fakeLastName() string {
	return pick(lastNames)
}

func fakePerson() string {
	return fakeFirstName() + " " + fakeLastName()
}

func fakeUsername() string {
	return strings.ToLower(fakeFirstName()+fakeLastName()) + randomDigits(2)
}

func fakeEmail() string {
	local := strings.ToLower(fakeFirstName() + "." + fakeLastName())
	return local + randomDigits(2) + "@" + pick(emailDomains)
}

func fakeCompany() string {
	return pick(companyNames) + " " + pick(companySuffixes)
}

func fakeDomain() string {
	return strings.ToLower(pick(companyNames)) + ".example.com"
}

func fakeCity() string {
	return pick(cities)
}

func fakeCountry() string {
	return pick(countries)
}

func fakePhone() string {
	return "+1-555-01" + randomDigits(2)
}

func fakeWord() string {
	return pick(words)
}

func fakeSentence() string {
	n := 4 + int(randUint64(5))
	out := make([]string, n)
	for i := range out {
		out[i] = pick(words)
	}
	s := strings.Join(out, " ")
	return strings.ToUpper(s[:1]) + s[1:] + "."
}
