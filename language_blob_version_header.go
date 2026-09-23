package gotreesitter

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// BlobRuntimeVersion is this runtime's current blob-format compatibility
// version. It increments whenever a change to the Language struct's gob
// encoding could produce silently incorrect (not just absent) behavior if
// decoded by an older runtime -- for example, adding a new *Certified
// capability flag whose zero value is unsafe to assume, the way an older
// runtime decoding a newer blob would. EncodeLanguageBlob stamps every newly
// written blob's MinRuntimeVersion with this constant.
//
// This is a distinct axis from LanguageVersion (the tree-sitter grammar ABI
// version, 13-15): LanguageVersion says whether the grammar rules a blob was
// compiled from are compatible with this parser; BlobRuntimeVersion says
// whether this runtime's Go struct layout is new enough to decode the blob's
// gob stream correctly.
const BlobRuntimeVersion uint32 = 1

// languageBlobVersionHeaderMagic identifies a version-header-wrapped blob. It
// is distinct from both the gzip magic (0x1f 0x8b) and languageBlobEnvelope's
// "GTSBLOB\0" magic, so a runtime that only understands one of the two wrapper
// formats fails closed (via gzip.NewReader or UnwrapLanguageBlobEnvelope)
// instead of silently misinterpreting the header bytes as blob content.
var languageBlobVersionHeaderMagic = [8]byte{'G', 'T', 'S', 'V', 'E', 'R', '1', 0}

const (
	languageBlobVersionHeaderSchemaVersionOffset       = len(languageBlobVersionHeaderMagic)
	languageBlobVersionHeaderMinRuntimeVersionOffset   = languageBlobVersionHeaderSchemaVersionOffset + 2
	languageBlobVersionHeaderGeneratorVersionLenOffset = languageBlobVersionHeaderMinRuntimeVersionOffset + 4
	languageBlobVersionHeaderFixedPrefixSize           = languageBlobVersionHeaderGeneratorVersionLenOffset + 2
	// languageBlobVersionHeaderSchemaVersion is the header layout version
	// (distinct from BlobRuntimeVersion, which is the Language struct's
	// compatibility version). It changes only if this header's own byte
	// layout changes.
	languageBlobVersionHeaderSchemaVersion = uint16(1)
)

// LanguageBlobInfo describes the version metadata recorded in a grammar
// blob's header, or the fact that the blob (or the in-memory Language) has
// none. See Language.BlobInfo.
type LanguageBlobInfo struct {
	// HasHeader is true when LoadLanguage found and validated a version
	// header on this blob.
	HasHeader bool
	// SchemaVersion is the blob header's own format version. Zero when
	// HasHeader is false.
	SchemaVersion uint16
	// GeneratorVersion identifies the tool and version that produced the
	// blob, e.g. "ts2go" or "grammargen". Empty when HasHeader is false or
	// the header did not set it.
	GeneratorVersion string
	// MinRuntimeVersion is the minimum BlobRuntimeVersion this runtime must
	// implement to load the blob correctly. Zero when HasHeader is false.
	MinRuntimeVersion uint32
}

// WrapLanguageBlobVersionHeader prepends a version header to payload (which
// may be a legacy gzip blob or an already-GTSBLOB-enveloped blob -- this
// wrapper is independent of, and layered outside, that inner envelope). The
// header records the blob schema version, the minimum runtime version
// required to load it, and an optional generator identity string.
func WrapLanguageBlobVersionHeader(payload []byte, minRuntimeVersion uint32, generatorVersion string) ([]byte, error) {
	if len(generatorVersion) > 0xFFFF {
		return nil, fmt.Errorf("wrap language blob version header: generator version string too long (%d bytes)", len(generatorVersion))
	}
	genBytes := []byte(generatorVersion)
	out := make([]byte, languageBlobVersionHeaderFixedPrefixSize+len(genBytes)+len(payload))
	copy(out, languageBlobVersionHeaderMagic[:])
	binary.BigEndian.PutUint16(out[languageBlobVersionHeaderSchemaVersionOffset:languageBlobVersionHeaderMinRuntimeVersionOffset], languageBlobVersionHeaderSchemaVersion)
	binary.BigEndian.PutUint32(out[languageBlobVersionHeaderMinRuntimeVersionOffset:languageBlobVersionHeaderGeneratorVersionLenOffset], minRuntimeVersion)
	binary.BigEndian.PutUint16(out[languageBlobVersionHeaderGeneratorVersionLenOffset:languageBlobVersionHeaderFixedPrefixSize], uint16(len(genBytes)))
	pos := languageBlobVersionHeaderFixedPrefixSize
	pos += copy(out[pos:], genBytes)
	copy(out[pos:], payload)
	return out, nil
}

// UnwrapLanguageBlobVersionHeader returns the inner payload and the header's
// declared version metadata as a LanguageBlobInfo. A blob with no recognized
// header magic is returned unchanged with info.HasHeader == false,
// preserving every wire format that predates this header. LoadLanguage calls
// this; a caller writing its own decoder around EncodeLanguageBlob's output
// (instead of calling LoadLanguage directly) needs it too, to stay in sync
// with the wire format and the runtime-version rejection.
func UnwrapLanguageBlobVersionHeader(data []byte) (payload []byte, info LanguageBlobInfo, err error) {
	if !bytes.HasPrefix(data, languageBlobVersionHeaderMagic[:]) {
		return data, LanguageBlobInfo{}, nil
	}
	if len(data) < languageBlobVersionHeaderFixedPrefixSize {
		return nil, LanguageBlobInfo{}, fmt.Errorf("unwrap language blob version header: truncated header: got %d bytes, need at least %d", len(data), languageBlobVersionHeaderFixedPrefixSize)
	}

	schemaVersion := binary.BigEndian.Uint16(data[languageBlobVersionHeaderSchemaVersionOffset:languageBlobVersionHeaderMinRuntimeVersionOffset])
	if schemaVersion != languageBlobVersionHeaderSchemaVersion {
		return nil, LanguageBlobInfo{}, fmt.Errorf("unwrap language blob version header: unsupported header schema version %d", schemaVersion)
	}
	minRuntimeVersion := binary.BigEndian.Uint32(data[languageBlobVersionHeaderMinRuntimeVersionOffset:languageBlobVersionHeaderGeneratorVersionLenOffset])
	genLen := int(binary.BigEndian.Uint16(data[languageBlobVersionHeaderGeneratorVersionLenOffset:languageBlobVersionHeaderFixedPrefixSize]))

	if len(data) < languageBlobVersionHeaderFixedPrefixSize+genLen {
		return nil, LanguageBlobInfo{}, fmt.Errorf("unwrap language blob version header: truncated generator version: got %d bytes, need at least %d", len(data), languageBlobVersionHeaderFixedPrefixSize+genLen)
	}
	generatorVersion := string(data[languageBlobVersionHeaderFixedPrefixSize : languageBlobVersionHeaderFixedPrefixSize+genLen])
	rest := data[languageBlobVersionHeaderFixedPrefixSize+genLen:]

	if minRuntimeVersion > BlobRuntimeVersion {
		return nil, LanguageBlobInfo{}, fmt.Errorf(
			"unwrap language blob version header: blob requires runtime version >= %d, this runtime implements version %d -- upgrade gotreesitter to load it",
			minRuntimeVersion, BlobRuntimeVersion)
	}

	return rest, LanguageBlobInfo{
		HasHeader:         true,
		SchemaVersion:     schemaVersion,
		MinRuntimeVersion: minRuntimeVersion,
		GeneratorVersion:  generatorVersion,
	}, nil
}
