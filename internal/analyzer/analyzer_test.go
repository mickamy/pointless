package analyzer_test

import (
	"testing"

	"github.com/mickamy/pointless/internal/analyzer"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	t.Parallel()

	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, analyzer.Analyzer, "a")
}
