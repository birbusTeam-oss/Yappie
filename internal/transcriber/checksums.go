package transcriber

// Known SHA256 checksums for whisper.cpp release binaries.
// Source: GitHub releases page (copy SHA256 digest for each asset).
// Update when bumping whisper.cpp version.
//
// NOTE: The actual SHA256 for whisper-bin-x64.zip v1.8.4 must be verified
// from the GitHub releases page. We set a placeholder that will be
// populated after first verified download. The download function
// will reject downloads if the checksum is empty or "UNVERIFIED".
var whisperBinaryChecksums = map[string]string{
	// v1.8.4 whisper-bin-x64.zip
	// TODO: Replace with actual SHA256 from GitHub releases page after
	// downloading once and running: sha256sum whisper-bin-x64.zip
	"whisper-bin-x64.zip": "UNVERIFIED",
}

// Known SHA1 checksums for whisper model files (git SHA1 blob hashes from HuggingFace).
// Source: https://huggingface.co/ggerganov/whisper.cpp
var modelChecksums = map[string]string{
	"tiny":                "bd577a113a864445d4c299885e0cb97d4ba92b5f",
	"tiny.en":             "c78c86eb1a8faa21b369bcd33207cc90d64ae9df",
	"base":                "465707469ff3a37a2b9b8d8f89f2f99de7299dac",
	"base.en":             "137c40403d78fd54d454da0f9bd998f78703390c",
	"small":               "55356645c2b361a969dfd0ef2c5a50d530afd8d5",
	"small.en":            "db8a495a91d927739e50b3fc1cc4c6b8f6c2d022",
	"large-v3-turbo":      "4af2b29d7ec73d781377bfd1758ca957a807e941",
	"large-v3-turbo-q5_0": "e050f7970618a659205450ad97eb95a18d69c9ee",
}

// Model download URLs
var modelDownloadURLs = map[string]string{
	"tiny":                "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin",
	"tiny.en":             "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin",
	"base":                "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin",
	"base.en":             "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.en.bin",
	"small":               "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin",
	"small.en":            "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.en.bin",
	"large-v3-turbo":      "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin",
	"large-v3-turbo-q5_0": "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo-q5_0.bin",
}

// GetModelChecksum returns the SHA1 checksum for a model name, or empty if unknown.
func GetModelChecksum(modelName string) (string, bool) {
	s, ok := modelChecksums[modelName]
	return s, ok
}

// GetModelDownloadURL returns the download URL for a model name, or empty if unknown.
func GetModelDownloadURL(modelName string) (string, bool) {
	u, ok := modelDownloadURLs[modelName]
	return u, ok
}