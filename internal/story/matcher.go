package story

import (
	"crypto/sha256"
	"encoding/hex"
	"hash/fnv"
	"math"
	"math/bits"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

const (
	MatchNone        = "none"
	MatchContentHash = "content_hash"
	MatchTitle       = "title"
	MatchHybrid      = "hybrid"
	MatchManual      = "manual"
)

var numberPattern = regexp.MustCompile(`\d+(?:\.\d+)?`)

type Features struct {
	NormalizedTitle string
	ContentHash     string
	ContentSimHash  uint64
	Embedding       []float32
	EmbeddingModel  string
}

type Result struct {
	Matched          bool
	Method           string
	FinalScore       float64
	EmbeddingScore   float64
	TitleScore       float64
	ContentScore     float64
	TimeScore        float64
	CriticalConflict bool
}

func BuildFeatures(title, content string) Features {
	normalizedTitle := normalizeTitle(title)
	normalizedContent := normalizeText(content)
	result := Features{
		NormalizedTitle: normalizedTitle,
		ContentSimHash:  simHash(normalizedContent),
	}
	if normalizedContent != "" {
		hash := sha256.Sum256([]byte(normalizedContent))
		result.ContentHash = hex.EncodeToString(hash[:])
	}
	return result
}

func Match(left, right Features, leftPublishedAt, rightPublishedAt time.Time) Result {
	embeddingScore := 0.0
	if left.EmbeddingModel != "" && left.EmbeddingModel == right.EmbeddingModel {
		embeddingScore = cosineSimilarity(left.Embedding, right.Embedding)
	}
	result := Result{
		Method:         MatchNone,
		EmbeddingScore: embeddingScore,
		TitleScore:     ngramSimilarity(left.NormalizedTitle, right.NormalizedTitle),
		ContentScore:   simHashSimilarity(left.ContentSimHash, right.ContentSimHash),
		TimeScore:      timeSimilarity(leftPublishedAt, rightPublishedAt),
	}
	result.CriticalConflict = criticalConflict(left.NormalizedTitle, right.NormalizedTitle)
	if result.CriticalConflict {
		return result
	}
	if left.ContentHash != "" && left.ContentHash == right.ContentHash {
		result.Matched = true
		result.Method = MatchContentHash
		result.FinalScore = 1
		return result
	}
	if result.TitleScore >= 0.92 && result.TimeScore >= 0.5 {
		result.Matched = true
		result.Method = MatchTitle
		result.FinalScore = result.TitleScore
		return result
	}

	weighted := result.TitleScore*0.25 + result.ContentScore*0.15 + result.TimeScore*0.05
	weight := 0.45
	if len(left.Embedding) > 0 && len(right.Embedding) > 0 &&
		left.EmbeddingModel != "" && left.EmbeddingModel == right.EmbeddingModel {
		weighted += result.EmbeddingScore * 0.55
		weight += 0.55
	}
	result.FinalScore = weighted / weight
	result.Matched = result.FinalScore >= 0.78 ||
		(result.EmbeddingScore >= 0.92 &&
			(result.TitleScore >= 0.1 || result.ContentScore >= 0.5) &&
			result.TimeScore >= 0.5)
	if result.Matched {
		result.Method = MatchHybrid
	}
	return result
}

func normalizeTitle(value string) string {
	for _, separator := range []string{"｜", "|"} {
		if before, _, ok := strings.Cut(value, separator); ok {
			value = before
		}
	}
	return normalizeText(value)
}

func normalizeText(value string) string {
	var result strings.Builder
	inTag := false
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		switch character {
		case '<':
			inTag = true
			continue
		case '>':
			inTag = false
			continue
		}
		if inTag || unicode.IsSpace(character) || unicode.IsPunct(character) || unicode.IsSymbol(character) {
			continue
		}
		result.WriteRune(character)
	}
	return result.String()
}

func simHash(value string) uint64 {
	grams := ngrams(value, 3)
	if len(grams) == 0 {
		return 0
	}
	var weights [64]int
	for _, gram := range grams {
		hasher := fnv.New64a()
		_, _ = hasher.Write([]byte(gram))
		hash := hasher.Sum64()
		for bit := range 64 {
			if hash&(uint64(1)<<bit) != 0 {
				weights[bit]++
			} else {
				weights[bit]--
			}
		}
	}
	var result uint64
	for bit, weight := range weights {
		if weight >= 0 {
			result |= uint64(1) << bit
		}
	}
	return result
}

func simHashSimilarity(left, right uint64) float64 {
	if left == 0 || right == 0 {
		return 0
	}
	return 1 - float64(bits.OnesCount64(left^right))/64
}

func ngramSimilarity(left, right string) float64 {
	leftGrams := ngrams(left, 2)
	rightGrams := ngrams(right, 2)
	if len(leftGrams) == 0 || len(rightGrams) == 0 {
		return 0
	}
	leftSet := make(map[string]struct{}, len(leftGrams))
	union := make(map[string]struct{}, len(leftGrams)+len(rightGrams))
	for _, gram := range leftGrams {
		leftSet[gram] = struct{}{}
		union[gram] = struct{}{}
	}
	intersection := 0
	for _, gram := range rightGrams {
		if _, ok := union[gram]; !ok {
			union[gram] = struct{}{}
		}
		if _, ok := leftSet[gram]; ok {
			intersection++
			delete(leftSet, gram)
		}
	}
	return float64(intersection) / float64(len(union))
}

func ngrams(value string, size int) []string {
	characters := []rune(value)
	if len(characters) == 0 {
		return nil
	}
	if len(characters) < size {
		return []string{value}
	}
	result := make([]string, 0, len(characters)-size+1)
	for index := 0; index <= len(characters)-size; index++ {
		result = append(result, string(characters[index:index+size]))
	}
	return result
}

func criticalConflict(left, right string) bool {
	leftNumbers := numberPattern.FindAllString(left, -1)
	rightNumbers := numberPattern.FindAllString(right, -1)
	slices.Sort(leftNumbers)
	slices.Sort(rightNumbers)
	return len(leftNumbers) > 0 && len(rightNumbers) > 0 && !slices.Equal(leftNumbers, rightNumbers)
}

func timeSimilarity(left, right time.Time) float64 {
	difference := left.Sub(right)
	if difference < 0 {
		difference = -difference
	}
	switch {
	case difference <= 6*time.Hour:
		return 1
	case difference <= 24*time.Hour:
		return 0.8
	case difference <= 72*time.Hour:
		return 0.5
	case difference <= 7*24*time.Hour:
		return 0.2
	default:
		return 0
	}
}

func cosineSimilarity(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for index := range left {
		l := float64(left[index])
		r := float64(right[index])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}
