package grpcx

import (
	"fmt"
	"math"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/bytesize"
	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/util"

	// Register gzip support. Requests use it only when grpc-compression is set.
	_ "google.golang.org/grpc/encoding/gzip"
)

type optionSettingKey string

const (
	optionSettingMaxRecvSize optionSettingKey = "grpc-max-recv-size"
	optionSettingMaxSendSize optionSettingKey = "grpc-max-send-size"
	optionSettingCompression optionSettingKey = "grpc-compression"
)

const compressionNone = "none"

var sizeSettings = []struct {
	key optionSettingKey
	set func(*Options, int)
}{
	{key: optionSettingMaxRecvSize, set: func(o *Options, size int) { o.MaxRecvSize = size }},
	{key: optionSettingMaxSendSize, set: func(o *Options, size int) { o.MaxSendSize = size }},
}

var compressors = map[string]string{
	compressionNone: "",
	"gzip":          "gzip",
}

func invalidSetting(key optionSettingKey, val, want string) error {
	msg := fmt.Sprintf("invalid %s %q (use %s)", key, val, want)
	if strings.TrimSpace(val) == "" {
		msg = fmt.Sprintf("missing %s value (use %s)", key, want)
	}
	return diag.New(diag.ClassProtocol, msg, grpcComponent)
}

// ApplyOptionSettings applies the gRPC settings recognized by the client.
func ApplyOptionSettings(opts *Options, settings map[string]string) error {
	return applyOptionSettings(opts, settings, true)
}

// In non-strict mode, invalid settings leave their current values unchanged.
func applyOptionSettings(opts *Options, settings map[string]string, strict bool) error {
	if opts == nil || len(settings) == 0 {
		return nil
	}

	norm := make(map[string]string, len(settings))
	for k, v := range settings {
		norm[util.LowerTrim(k)] = v
	}

	for _, s := range sizeSettings {
		val, ok := norm[string(s.key)]
		if !ok {
			continue
		}
		switch size, err := parseMessageSize(val); {
		case err == nil:
			s.set(opts, size)
		case strict:
			return invalidSetting(s.key, val, "a size such as 8MB")
		}
	}

	if val, ok := norm[string(optionSettingCompression)]; ok {
		name, valid := compressors[util.LowerTrim(val)]
		switch {
		case valid:
			opts.Compression = name
		case strict:
			return invalidSetting(optionSettingCompression, val, "gzip or none")
		}
	}

	return nil
}

func parseMessageSize(raw string) (int, error) {
	size, err := bytesize.Parse(raw)
	if err != nil {
		return 0, err
	}
	if size <= 0 || size > math.MaxInt32 {
		return 0, fmt.Errorf("size %q is out of range", raw)
	}
	return int(size), nil
}
