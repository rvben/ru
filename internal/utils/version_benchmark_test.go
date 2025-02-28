package utils

import (
	"testing"
)

var benchmarkVersions = []string{
	"1.0.0",
	"2.31.0",
	"3.14.15",
	"10.20.30",
	"0.0.1",
	"1.2.3-alpha",
	"2.0.0-beta.1",
	"3.0.0-rc.2",
	"4.5.6+build.123",
	"1.2.3-alpha+build.123",
}

var benchmarkConstraints = []string{
	"==1.0.0",
	">=2.0.0",
	"<=3.0.0",
	">1.0.0",
	"<3.0.0",
	"~=2.0.0",
	"^1.0.0",
	">=1.0.0,<2.0.0",
	">=1.0.0,<=2.0.0,!=1.5.0",
	"1.0.0",
}

// BenchmarkParseVersion benchmarks version parsing performance
func BenchmarkParseVersion(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ParseVersion(benchmarkVersions[i%len(benchmarkVersions)])
	}
}

// BenchmarkParseVersionWithCache benchmarks version parsing with cache
func BenchmarkParseVersionWithCache(b *testing.B) {
	b.ReportAllocs()
	// Warm up the cache
	for _, v := range benchmarkVersions {
		ParseVersion(v)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseVersion(benchmarkVersions[i%len(benchmarkVersions)])
	}
}

// BenchmarkClearCache benchmarks cache clearing
func BenchmarkClearCache(b *testing.B) {
	b.ReportAllocs()
	// Fill the cache
	for _, v := range benchmarkVersions {
		ParseVersion(v)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ClearVersionCache()
	}
}

// BenchmarkVersionCompare benchmarks version comparison
func BenchmarkVersionCompare(b *testing.B) {
	b.ReportAllocs()
	versions := make([]*Version, len(benchmarkVersions))
	for i, v := range benchmarkVersions {
		versions[i] = ParseVersion(v)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v1 := versions[i%len(versions)]
		v2 := versions[(i+1)%len(versions)]
		v1.Compare(v2)
	}
}

// BenchmarkVersionIsGreaterThan benchmarks version greater than comparison
func BenchmarkVersionIsGreaterThan(b *testing.B) {
	b.ReportAllocs()
	versions := make([]*Version, len(benchmarkVersions))
	for i, v := range benchmarkVersions {
		versions[i] = ParseVersion(v)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v1 := versions[i%len(versions)]
		v2 := versions[(i+1)%len(versions)]
		v1.IsGreaterThan(v2)
	}
}

// BenchmarkVersionIsCompatible benchmarks constraint compatibility check
func BenchmarkVersionIsCompatible(b *testing.B) {
	b.ReportAllocs()
	versions := make([]*Version, len(benchmarkVersions))
	for i, v := range benchmarkVersions {
		versions[i] = ParseVersion(v)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := versions[i%len(versions)]
		constraint := benchmarkConstraints[i%len(benchmarkConstraints)]
		v.IsCompatible(constraint)
	}
}

// BenchmarkVersionIsCompatibleComplex benchmarks complex constraint compatibility check
func BenchmarkVersionIsCompatibleComplex(b *testing.B) {
	b.ReportAllocs()
	versions := make([]*Version, len(benchmarkVersions))
	for i, v := range benchmarkVersions {
		versions[i] = ParseVersion(v)
	}
	// Only test complex constraints with commas
	complexConstraints := []string{
		">=1.0.0,<2.0.0",
		">=1.0.0,<=2.0.0,!=1.5.0",
		"==1.0.0,<3.0.0",
		">=2.0.0,<3.0.0,!=2.5.0",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := versions[i%len(versions)]
		constraint := complexConstraints[i%len(complexConstraints)]
		v.IsCompatible(constraint)
	}
}
