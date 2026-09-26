//go:build goexperiment.simd

package dataset

import "simd/archsimd"

// packedDistanceKey usa AVX2 quando a toolchain Go experimental SIMD consegue
// gerar o caminho amd64. Os campos opcionais -1 são convertidos para a mesma
// coordenada virtual -PackedScale usada pelo cálculo escalar, então o
// resultado permanece exatamente igual ao fallback.
func packedDistanceKey(left, right PackedVector) uint64 {
	var sum uint64

	// As dimensões 0..3 e 8..11 nunca aceitam o sentinela. Mantê-las em
	// uint32 deixa o quadrado abaixo de 2^32 e usa somente vetores de 128 bits,
	// compatíveis com máquinas AVX2 sem exigir AVX-512.
	sum += packedDistanceChunk4Normal(left[:], right[:])
	sum += packedDistanceChunk4Normal(left[8:], right[8:])

	// As dimensões 4..7 incluem os dois campos opcionais (-1), então o caminho
	// escalar preserva a distância especial sem ampliar o vetor SIMD.
	for dimension := 4; dimension < 8; dimension++ {
		difference := packedDifferenceKey(left[dimension], right[dimension])
		sum += uint64(difference * difference)
	}

	// As duas dimensões restantes nunca aceitam o sentinela e são poucas o
	// suficiente para não justificar outra carga SIMD parcial.
	for dimension := 12; dimension < len(left); dimension++ {
		difference := packedDifferenceKey(left[dimension], right[dimension])
		sum += uint64(difference * difference)
	}
	return sum
}

func packedDistanceChunk4Normal(left, right []uint16) uint64 {
	leftValue := archsimd.LoadUint16x8SlicePart(left).ExtendLo4ToUint32()
	rightValue := archsimd.LoadUint16x8SlicePart(right).ExtendLo4ToUint32()

	difference := leftValue.Max(rightValue).Sub(leftValue.Min(rightValue))
	squared := difference.Mul(difference)
	var lanes32 [4]uint32
	squared.Store(&lanes32)
	var sum uint64
	for _, value := range lanes32 {
		sum += uint64(value)
	}
	return sum
}
