//go:build !goexperiment.simd

package dataset

// packedDistanceKey calcula a distância no caminho compatível com qualquer
// toolchain Go. O arquivo SIMD fornece uma implementação com a mesma
// assinatura quando GOEXPERIMENT=simd está ativo.
func packedDistanceKey(left, right PackedVector) uint64 {
	var sum uint64
	for dimension := range left {
		difference := packedDifferenceKey(left[dimension], right[dimension])
		sum += uint64(difference * difference)
	}
	return sum
}
