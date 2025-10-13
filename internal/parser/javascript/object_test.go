package js

import "testing"

func TestFormatValueHandlesExtendedSyntax(t *testing.T) {
	input := `{
  // trailing comma and single quotes
  count: 0x1f,
  label: 'hello',
  list: [1, 2,],
  nan: NaN,
  negInf: -Infinity,
}`

	got, err := FormatValue(input)
	if err != nil {
		t.Fatalf("FormatValue returned error: %v", err)
	}

	want := `{
  count: 0x1f,
  label: "hello",
  list: [
    1,
    2
  ],
  nan: NaN,
  negInf: -Infinity
}`

	if got != want {
		t.Fatalf("unexpected formatted output\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestFormatInlineValueRespectsIndent(t *testing.T) {
	input := `{
  "token": "value",
}`

	got, ok := FormatInlineValue(input, 1)
	if !ok {
		t.Fatalf("FormatInlineValue returned !ok")
	}

	want := `{
    token: "value"
  }`

	if got != want {
		t.Fatalf("unexpected inline format\nwant:\n%q\n\ngot:\n%q", want, got)
	}
}

func TestFormatValueRejectsUnknownEscape(t *testing.T) {
	input := `{ value: "bad\q" }`
	if _, err := FormatValue(input); err == nil {
		t.Fatalf("expected error for unknown escape, got nil")
	}
}

func TestFormatValueRejectsUnterminatedBlockComment(t *testing.T) {
	input := `{
  /* missing end
  value: 1
}`
	if _, err := FormatValue(input); err == nil {
		t.Fatalf("expected error for unterminated block comment, got nil")
	}
}

func TestFormatValueRejectsInvalidSignedLiteral(t *testing.T) {
	if _, err := FormatValue(`+Infinityfoo`); err == nil {
		t.Fatalf("expected error for invalid signed literal, got nil")
	}
}

func TestFormatValueRejectsBadNumericSeparator(t *testing.T) {
	if _, err := FormatValue(`{ value: 1__0 }`); err == nil {
		t.Fatalf("expected error for invalid numeric separator, got nil")
	}

	if _, err := FormatValue(`0x_FF`); err == nil {
		t.Fatalf("expected error for invalid numeric separator after base prefix, got nil")
	}
}

func TestFormatValueAllowsNumericSeparators(t *testing.T) {
	input := `{
  dec: 1_234,
  bin: 0b1010_0001,
  hex: 0xDE_AD,
}`

	got, err := FormatValue(input)
	if err != nil {
		t.Fatalf("FormatValue returned error: %v", err)
	}

	want := `{
  bin: 0b1010_0001,
  dec: 1234,
  hex: 0xDEAD
}`

	if got != want {
		t.Fatalf("unexpected formatted output\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}
