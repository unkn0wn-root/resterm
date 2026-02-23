package generator

const (
	authTypeBasic  = "basic"
	authTypeBearer = "bearer"
	authTypeOAuth2 = "oauth2"
	authTypeAPIKey = "apikey"

	defaultSampleValue = "sample"

	styleDeepObjectTok     = "deepobject"
	styleSpaceDelimitedTok = "spacedelimited"
	stylePipeDelimitedTok  = "pipedelimited"
)

type pSty uint8

const (
	pStyUnk pSty = iota
	pStyForm
	pStySimple
	pStyDeepObj
	pStySpaceDel
	pStyPipeDel
)

func varRef(name string) string {
	return "{{" + name + "}}"
}
