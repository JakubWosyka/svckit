package msgs

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/minus5/svckit/pkg/compress"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	smallBody = []byte(`{"oid":"1m1"}`)
	largeBody = bytes.Repeat([]byte(`{"oid":"1m1","odds":1.25},`), 2000)
)

func header(t *testing.T, msg []byte) map[string]interface{} {
	t.Helper()
	i := bytes.IndexByte(msg, HeaderSeparator[0])
	require.True(t, i > 0, "no header separator")
	h := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(msg[:i], &h))
	return h
}

func TestCreateBackendGzipUnchanged(t *testing.T) {
	want := append([]byte(`{"encoding":"gzip","no":7,"ts":123,"type":"fixtureOffer"}`+"\n"), compress.Gzip(largeBody)...)
	assert.Equal(t, want, CreateBackendTs("fixtureOffer", 7, 123, largeBody))

	want = append([]byte(`{"no":7,"ts":123,"type":"fixtureOffer"}`+"\n"), smallBody...)
	assert.Equal(t, want, CreateBackendTs("fixtureOffer", 7, 123, smallBody))
}

func TestCreateBackendEncoded(t *testing.T) {
	for _, tc := range []struct {
		name     string
		encoding compress.Encoding
		body     []byte
		want     compress.Encoding
	}{
		{"zstd large", compress.EncodingZstd, largeBody, compress.EncodingZstd},
		{"gzip large", compress.EncodingGzip, largeBody, compress.EncodingGzip},
		{"none large", compress.EncodingNone, largeBody, compress.EncodingNone},
		{"zstd small", compress.EncodingZstd, smallBody, compress.EncodingNone},
		{"unknown large", compress.Encoding("br"), largeBody, compress.EncodingNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := CreateBackendEncoded("fixtureOffer", 7, tc.body, tc.encoding)
			h := header(t, msg)
			if tc.want == compress.EncodingNone {
				assert.NotContains(t, h, "encoding")
			} else {
				assert.Equal(t, string(tc.want), h["encoding"])
			}

			b, err := NewBackend(msg)
			require.NoError(t, err)
			assert.Equal(t, tc.body, b.Body)
			assert.Equal(t, tc.want, compress.Detect(b.RawBody))
			assert.Equal(t, string(tc.want), b.Encoding)
		})
	}
}

func TestBackendFactoryEncoding(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []func(map[string]interface{})
		want compress.Encoding
	}{
		{"default gzip", nil, compress.EncodingGzip},
		{"NoGzip", []func(map[string]interface{}){NoGzip()}, compress.EncodingNone},
		{"WithEncoding zstd", []func(map[string]interface{}){WithEncoding(compress.EncodingZstd)}, compress.EncodingZstd},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]func(map[string]interface{}){Header("id", "x")}, tc.opts...)
			msg := BackendFactory("fixtureOffer", largeBody, opts...)
			h := header(t, msg)
			assert.Equal(t, "x", h["id"])
			assert.NotContains(t, h, encodingKey)
			if tc.want == compress.EncodingNone {
				assert.NotContains(t, h, "encoding")
			} else {
				assert.Equal(t, string(tc.want), h["encoding"])
			}
			b, err := NewBackend(msg)
			require.NoError(t, err)
			assert.Equal(t, largeBody, b.Body)
			assert.Equal(t, tc.want, compress.Detect(b.RawBody))
		})
	}
}

func TestRepackKeepsEncodedBody(t *testing.T) {
	msg := CreateBackendEncoded("fixtureOffer", 7, largeBody, compress.EncodingZstd)
	b, err := NewBackend(msg)
	require.NoError(t, err)
	raw := append([]byte{}, b.RawBody...)

	require.True(t, b.SetDc("dc1"))
	out := b.RawMessage()
	h := header(t, out)
	assert.Equal(t, "zstd", h["encoding"])
	assert.Equal(t, "dc1", h["dc"])

	b2, err := NewBackend(out)
	require.NoError(t, err)
	assert.Equal(t, raw, b2.RawBody)
	assert.Equal(t, largeBody, b2.Body)
}

func TestEncodingSizeLimit(t *testing.T) {
	for _, enc := range []compress.Encoding{compress.EncodingGzip, compress.EncodingZstd} {
		atLimit := bytes.Repeat([]byte("a"), GzipMsgSizeLimit)
		overLimit := bytes.Repeat([]byte("a"), GzipMsgSizeLimit+1)

		assert.NotContains(t, header(t, CreateBackendEncoded("x", 1, atLimit, enc)), "encoding")
		assert.NotContains(t, header(t, BackendFactory("x", atLimit, WithEncoding(enc))), "encoding")
		assert.Equal(t, string(enc), header(t, CreateBackendEncoded("x", 1, overLimit, enc))["encoding"])
		assert.Equal(t, string(enc), header(t, BackendFactory("x", overLimit, WithEncoding(enc)))["encoding"])
	}
}
