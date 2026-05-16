use rand::SeedableRng;
use rand_distr::{Distribution, Normal};

const THETA: f64     = 0.1;
const BASE_LOAD: f64 = 15_000.0;
const LOAD_VOL: f64  = 500.0;
const LMP_MIN: f64   = 1.0;
const LMP_MAX: f64   = 499.0;
const LOAD_MIN: f64  = 5_001.0;

pub struct TickGenerator {
    base_lmp:   f64,
    volatility: f64,
    lmp:        f64,
    rng:        rand::rngs::SmallRng,
    dist:       Normal<f64>,
}

impl TickGenerator {
    pub fn new(base_lmp: f64, volatility: f64, seed: u32) -> Self {
        Self {
            base_lmp,
            volatility,
            lmp: base_lmp,
            rng: rand::rngs::SmallRng::seed_from_u64(seed as u64),
            dist: Normal::new(0.0, 1.0).unwrap(),
        }
    }

    pub fn next(&mut self, out_lmp: &mut f64, out_load_mw: &mut f64) {
        self.lmp += THETA * (self.base_lmp - self.lmp)
            + self.volatility * self.dist.sample(&mut self.rng);
        self.lmp = self.lmp.clamp(LMP_MIN, LMP_MAX);
        *out_lmp = self.lmp;
        let load = BASE_LOAD + LOAD_VOL * self.dist.sample(&mut self.rng);
        *out_load_mw = load.max(LOAD_MIN);
    }
}

// --- extern "C" ABI — same symbols as the former C++ libmarketdata.so ---

#[no_mangle]
pub extern "C" fn tick_generator_create(
    base_lmp: f64,
    volatility: f64,
    seed: u32,
) -> *mut TickGenerator {
    Box::into_raw(Box::new(TickGenerator::new(base_lmp, volatility, seed)))
}

#[no_mangle]
pub extern "C" fn tick_generator_destroy(gen: *mut TickGenerator) {
    if !gen.is_null() {
        unsafe { drop(Box::from_raw(gen)); }
    }
}

#[no_mangle]
pub extern "C" fn tick_generator_next(
    gen: *mut TickGenerator,
    out_lmp: *mut f64,
    out_load_mw: *mut f64,
) {
    unsafe { (*gen).next(&mut *out_lmp, &mut *out_load_mw); }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn lmp_stays_in_range() {
        let mut g = TickGenerator::new(45.0, 5.0, 42);
        for _ in 0..1000 {
            let (mut lmp, mut load) = (0.0f64, 0.0f64);
            g.next(&mut lmp, &mut load);
            assert!(lmp >= 1.0 && lmp <= 499.0, "lmp={lmp} out of [1,499]");
            assert!(load > 5000.0, "load={load} not > 5000");
        }
    }

    #[test]
    fn deterministic_with_same_seed() {
        let mut g1 = TickGenerator::new(45.0, 5.0, 42);
        let mut g2 = TickGenerator::new(45.0, 5.0, 42);
        for i in 0..20 {
            let (mut l1, mut d1, mut l2, mut d2) = (0.0f64, 0.0f64, 0.0f64, 0.0f64);
            g1.next(&mut l1, &mut d1);
            g2.next(&mut l2, &mut d2);
            assert_eq!(l1, l2, "lmp diverged at tick {i}");
        }
    }
}
