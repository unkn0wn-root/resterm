package curl

const (
	cmdCurl    = "curl"
	cmdSudo    = "sudo"
	cmdEnv     = "env"
	cmdCommand = "command"
	cmdTime    = "time"
	cmdNoGlob  = "noglob"
)

var promptPrefixes = []string{"$", "%", ">", "!"}

const (
	headerContentType    = "Content-Type"
	headerAcceptEncoding = "Accept-Encoding"
	headerAuthorization  = "Authorization"
	headerUserAgent      = "User-Agent"
	headerReferer        = "Referer"
	headerCookie         = "Cookie"
)

const (
	mimeJSON              = "application/json"
	mimeFormURLEncoded    = "application/x-www-form-urlencoded"
	mimeMultipartForm     = "multipart/form-data"
	mimeOctetStream       = "application/octet-stream"
	acceptEncodingDefault = "gzip, deflate, br"
)

const (
	multipartBoundaryDefault = "resterm-boundary"
	multipartBoundaryPrefix  = "resterm-"
	boundaryHashLength       = 12 // bytes of SHA-256 used for multipart boundary
	urlQuoteChars            = "\"'"
	authTypeBasic            = "basic"
	authHeaderBasicPrefix    = "Basic "
	shortOptTokenLen         = 2 // length of "-x" when the value is separate
)
