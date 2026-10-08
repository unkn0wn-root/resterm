# Standard library

The standard library provides helpers for text, JSON, lists, time, crypto, and other request tasks. It is available as `rts`, and `stdlib` still works as a deprecated alias. The library does not allow file writes or network access.

Core helpers and namespaces (`crypto`, `base64`, `url`, `time`, `json`, `headers`, `query`, `encoding`) are also available at the top level. The `text`, `list`, `dict`, and `math` namespaces are available only under `rts`.

Builtins and reserved words may be removed within a major version, but only after deprecation in a minor release. Removal happens no earlier than the next minor release. Until then, the builtin still works and the parser warns on each line that uses it (`WARN line <n>` in the status bar, with the full text in Explain). Release notes list each removal and its replacement. See [Compatibility](../compatibility.md) for the wider versioning policy.

## Core helpers

- `rts.fail(msg)` stops evaluation and returns an error message.
- `rts.len(x)` returns the length of a string, list, or dict.
- `rts.contains(container, value)` applies the same membership relation as `value in container`.
- `rts.match(pattern, text)` applies a regular expression to text and returns true when it matches.
- `rts.str(x)` converts a value to a string, using JSON for lists and dicts.
- `rts.num(x[, def])` converts a value to a number, or returns `def` when conversion fails.
- `rts.int(x[, def])` converts a value to an integer, or returns `def` when conversion fails.
- `rts.bool(x[, def])` converts a value to a bool, or returns `def` when conversion fails.
- `rts.typeof(x)` returns the type name.
- `rts.uuid()` generates a UUID and requires random generation to be enabled.

## Crypto helpers

- `rts.crypto.sha256(text)` returns a hex-encoded SHA-256 digest.
- `rts.crypto.hmacSha256(key, text)` returns a hex-encoded HMAC-SHA256 digest.
- `rts.crypto.sha512(text)` returns a hex-encoded SHA-512 digest.
- `rts.crypto.hmacSha512(key, text)` returns a hex-encoded HMAC-SHA512 digest.

## Encoding and URL helpers

- `rts.base64.encode(x)` encodes a string to base64.
- `rts.base64.decode(x)` decodes a base64 string.
- `rts.encoding.hex.encode(x)` encodes a string to hex.
- `rts.encoding.hex.decode(x)` decodes a hex string.
- `rts.encoding.base64url.encode(x)` encodes a string to base64url (no padding).
- `rts.encoding.base64url.decode(x)` decodes a base64url string.
- `rts.url.encode(x)` percent-encodes a string for use in a URL.
- `rts.url.decode(x)` decodes a percent-encoded string.

## Time helpers

- `rts.time.nowISO()` returns the current time in ISO 8601 format.
- `rts.time.nowUnix()` returns the current time as unix seconds.
- `rts.time.nowUnixString()` returns the current time as a decimal unix seconds string.
- `rts.time.nowUnixMs()` returns the current time as unix milliseconds.
- `rts.time.format(layout)` formats the current time with the given layout string.
- `rts.time.parse(layout, value)` parses a time string and returns unix seconds (fractional).
- `rts.time.formatUnix(ts, layout)` formats a unix timestamp with the given layout.
- `rts.time.addUnix(ts, secondsOrDuration)` adds seconds (number) or a duration string to a unix timestamp.
- `rts.time.duration(value)` parses a duration string (including `d` and `w`) and returns seconds.

## JSON helpers

- `rts.json.file(path)` reads and parses JSON using the request base directory (only when file access is enabled).
- `rts.json.parse(text)` parses a JSON string into RestermScript values.
- `rts.json.stringify(value[, indent])` converts a value to JSON text. `indent` can be a string or a number (0-32).
- `rts.json.get(value[, path])` returns the value at a dot or `[index]` path (optional leading `$`) and returns null when missing. Quote a key that contains dots or spaces, as in `$["display.name"]` or `['a b']`.
- `rts.json.has(value, path)` returns true when a value exists at the path.

## Text helpers

- `rts.text.lower(s)` returns a lowercased string.
- `rts.text.upper(s)` returns an uppercased string.
- `rts.text.trim(s)` trims leading and trailing whitespace.
- `rts.text.split(s, sep)` splits a string into a list.
- `rts.text.join(list, sep)` joins list items with a separator (items may be strings, numbers, or bools).
- `rts.text.replace(s, old, new)` replaces all occurrences of `old` with `new`.
- `rts.text.startsWith(s, prefix)` returns true when a string starts with `prefix`.
- `rts.text.endsWith(s, suffix)` returns true when a string ends with `suffix`.

## List helpers

- `rts.list.append(list, item)` returns a new list with `item` appended.
- `rts.list.concat(a, b)` returns a new list with `b` appended to `a`.
- `rts.list.sort(list)` returns a sorted copy (numbers or strings only).
- `rts.list.map(list, fn)` returns a new list with `fn(item)` applied to each value.
- `rts.list.filter(list, fn)` returns a new list of values where `fn(item)` is truthy.
- `rts.list.any(list, fn)` returns true if any value makes `fn(item)` truthy.
- `rts.list.all(list, fn)` returns true if all values make `fn(item)` truthy.
- `rts.list.slice(list, start[, end])` returns a slice of the list.
- `rts.list.unique(list)` returns a list of unique primitive values.

## Dict helpers

- `rts.dict.keys(dict)` returns a sorted list of keys.
- `rts.dict.values(dict)` returns values ordered by sorted keys.
- `rts.dict.items(dict)` returns a list of `{key, value}` entries ordered by key.
- `rts.dict.set(dict, key, value)` returns a new dict with `key` set.
- `rts.dict.merge(a, b)` returns a new dict with `b` applied over `a`.
- `rts.dict.remove(dict, key)` returns a new dict without `key`.
- `rts.dict.get(dict, key[, def])` returns `def` or null when missing.
- `rts.dict.has(dict, key)` returns true when a key exists.
- `rts.dict.pick(dict, keys)` returns a dict with the specified keys.
- `rts.dict.omit(dict, keys)` returns a dict without the specified keys.

These helpers use keys exactly as written, like `dict[key]`. See [Keys and names](language.md#keys-and-names).

## Header and query helpers

- `headers.get(h, name)` returns the first value of a header, or null when it is missing.
- `headers.has(h, name)` returns true when a header carries a value.
- `headers.set(h, name, value)` returns a new dict with a string or `list<string>` set, replacing the header regardless of name casing.
- `headers.remove(h, name)` returns a new dict without the header, regardless of name casing.
- `headers.merge(a, b)` returns a new dict with `b` applied over `a`. A null value in `b` removes that header.
- `headers.normalize(h)` returns a new dict with the names lowercased.
- `query.parse(rawQuery)` parses raw query text. It never guesses that its argument is a URL.
- `query.fromURL(url)` parses the query component of a URL.
- `query.encode(query)` encodes a query multimap into a query string.
- `query.merge(url, query)` returns the URL with the parameters applied. Null or an empty list removes a parameter.

Header and query dictionaries use `dict<string, string | list<string>>`. One value is a string, multiple values are a list, and zero values are an empty list. This depends on the number of values, so even a one-element input list is returned as a string.

These helpers do not convert numbers or booleans to strings for you. Null is not a stored value; it only marks a value for removal in a patch passed to `headers.merge` or `query.merge`.

Header names are case-insensitive HTTP field names. Two forms of the same header always return an error, because picking one would depend on map order. Every header helper validates the entire input block and the requested name. Returned header names are lowercased, and `headers.get` returns the first value whether the stored value is a string or a list.

Query helpers keep keys and values exactly as written, including empty keys and whitespace. Encoding preserves the data but may change order and escaping. `query.parse` removes one leading `?` as syntax and treats every other byte as query data. `query.fromURL` and `query.merge` do not trim or repair their URL argument. Call `rts.text.trim` yourself if you want trimming.

## Math helpers

- `rts.math.abs(x)` returns the absolute value.
- `rts.math.min(a, b)` returns the smaller value.
- `rts.math.max(a, b)` returns the larger value.
- `rts.math.clamp(x, min, max)` clamps `x` into the range.
- `rts.math.floor(x)` returns the largest integer <= x.
- `rts.math.ceil(x)` returns the smallest integer >= x.
- `rts.math.round(x)` rounds to the nearest integer (half away from zero).
