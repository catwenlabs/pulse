package story

import (
	"math"
	"testing"
	"time"
)

func TestFeaturesNormalizeAndFingerprintContent(t *testing.T) {
	got := BuildFeatures("  OpenAI 发布新模型｜示例媒体 ", "<p>模型能力显著提升。</p>")

	if got.NormalizedTitle != "openai发布新模型" {
		t.Errorf("NormalizedTitle = %q", got.NormalizedTitle)
	}
	if got.ContentHash == "" {
		t.Fatal("ContentHash is empty")
	}
	if got.ContentSimHash == 0 {
		t.Fatal("ContentSimHash is zero")
	}
}

func TestFeaturesDoNotFingerprintMissingContent(t *testing.T) {
	got := BuildFeatures("只有标题", "")
	if got.ContentHash != "" {
		t.Fatalf("ContentHash = %q, want empty", got.ContentHash)
	}
}

func TestMatchExactContentWithoutEmbedding(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	left := BuildFeatures("OpenAI 发布新模型", "<p>OpenAI 今天发布了新模型。</p>")
	right := BuildFeatures("OpenAI正式发布新模型", "<p>OpenAI 今天发布了新模型。</p>")

	result := Match(left, right, now, now.Add(2*time.Hour))

	if !result.Matched || result.Method != MatchContentHash {
		t.Fatalf("Match() = %+v", result)
	}
	if result.FinalScore != 1 {
		t.Errorf("FinalScore = %v, want 1", result.FinalScore)
	}
}

func TestMatchUsesEmbeddingWithTraditionalSignals(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	left := BuildFeatures("某公司宣布裁员一万人", "公司将削减全球约百分之十的岗位。")
	right := BuildFeatures("该公司计划削减约10%的全球岗位", "此次调整预计涉及约一万名员工。")
	left.Embedding = []float32{1, 0, 0}
	right.Embedding = []float32{0.99, 0.02, 0}
	left.EmbeddingModel = "test-model"
	right.EmbeddingModel = "test-model"

	result := Match(left, right, now, now.Add(time.Hour))

	if !result.Matched {
		t.Fatalf("Match() = %+v, want matched", result)
	}
	if result.EmbeddingScore < 0.99 {
		t.Errorf("EmbeddingScore = %v", result.EmbeddingScore)
	}
}

func TestMatchRejectsCriticalNumberConflict(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	left := BuildFeatures("公司发布 2025 年年度报告", "公司公布2025年年度报告。")
	right := BuildFeatures("公司发布 2026 年年度报告", "公司公布2026年年度报告。")
	left.Embedding = []float32{1, 0}
	right.Embedding = []float32{1, 0}

	result := Match(left, right, now, now)

	if result.Matched || !result.CriticalConflict {
		t.Fatalf("Match() = %+v, want critical conflict", result)
	}
}

func TestMatchRejectsCriticalDirectionConflict(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	left := BuildFeatures("美联储宣布加息 50 个基点", "")
	right := BuildFeatures("美联储宣布降息 50 个基点", "")
	left.Embedding = []float32{1, 0}
	right.Embedding = []float32{1, 0}

	result := Match(left, right, now, now)

	if result.Matched || !result.CriticalConflict {
		t.Fatalf("Match() = %+v, want critical conflict on opposite directions", result)
	}
}

func TestMatchAggregatesByCanonicalURL(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	left := BuildFeatures("媒体一报道同一事件", "")
	right := BuildFeatures("另一家媒体转载了该事件", "")
	left.CanonicalURL = "https://example.com/article-a"
	right.CanonicalURL = "https://example.com/article-a"
	left.Embedding = []float32{1, 0}
	right.Embedding = []float32{0, 1}

	result := Match(left, right, now, now)

	if !result.Matched || result.Method != MatchURL || result.FinalScore != 1 {
		t.Fatalf("Match() = %+v, want canonical URL match", result)
	}
}

func TestMatchRejectsCriticalConflictDespiteCanonicalURL(t *testing.T) {
	now := time.Date(2026, 7, 30, 8, 0, 0, 0, time.UTC)
	left := BuildFeatures("公司发布 2025 年年度报告", "")
	right := BuildFeatures("公司发布 2026 年年度报告", "")
	left.CanonicalURL = "https://example.com/report"
	right.CanonicalURL = "https://example.com/report"
	left.Embedding = []float32{1, 0}
	right.Embedding = []float32{1, 0}

	result := Match(left, right, now, now)

	if result.Matched || !result.CriticalConflict {
		t.Fatalf("Match() = %+v, want critical conflict to override canonical URL", result)
	}
}

func TestCosineSimilarity(t *testing.T) {
	if got := cosineSimilarity([]float32{1, 1}, []float32{1, 1}); math.Abs(got-1) > 1e-6 {
		t.Errorf("cosineSimilarity() = %v", got)
	}
	if got := cosineSimilarity([]float32{1}, []float32{1, 2}); got != 0 {
		t.Errorf("dimension mismatch = %v", got)
	}
}
