package distuv

import "math"

type ChiSquared struct {
	K float64
}

func (c ChiSquared) Survival(x float64) float64 {
	if c.K <= 0 || math.IsNaN(c.K) || math.IsNaN(x) {
		return math.NaN()
	}
	if x <= 0 {
		return 1
	}
	return regularizedGammaQ(c.K/2, x/2)
}

func regularizedGammaQ(a, x float64) float64 {
	if a <= 0 || x < 0 {
		return math.NaN()
	}
	if x == 0 {
		return 1
	}
	if x < a+1 {
		return 1 - regularizedGammaPSeries(a, x)
	}
	return regularizedGammaQContinuedFraction(a, x)
}

func regularizedGammaPSeries(a, x float64) float64 {
	const (
		epsilon = 1e-14
		maxIter = 10000
	)
	sum := 1 / a
	delta := sum
	ap := a
	for i := 0; i < maxIter; i++ {
		ap++
		delta *= x / ap
		sum += delta
		if math.Abs(delta) < math.Abs(sum)*epsilon {
			break
		}
	}
	lgamma, _ := math.Lgamma(a)
	return sum * math.Exp(-x+a*math.Log(x)-lgamma)
}

func regularizedGammaQContinuedFraction(a, x float64) float64 {
	const (
		epsilon = 1e-14
		fpmin   = 1e-300
		maxIter = 10000
	)
	b := x + 1 - a
	c := 1 / fpmin
	d := b
	if math.Abs(d) < fpmin {
		d = fpmin
	}
	d = 1 / d
	h := d
	for i := 1; i <= maxIter; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < fpmin {
			d = fpmin
		}
		c = b + an/c
		if math.Abs(c) < fpmin {
			c = fpmin
		}
		d = 1 / d
		delta := d * c
		h *= delta
		if math.Abs(delta-1) < epsilon {
			break
		}
	}
	lgamma, _ := math.Lgamma(a)
	return math.Exp(-x+a*math.Log(x)-lgamma) * h
}
