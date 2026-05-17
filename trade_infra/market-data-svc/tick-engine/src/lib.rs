use rand::SeedableRng;
use rand_distr::{Distribution, Normal};

const THETA: f64     = 0.1;
const BASE_LOAD: f64 = 15_000.0;
const LOAD_VOL: f64  = 500.0;
const LMP_MIN: f64   = 1.0;
const LMP_MAX: f64   = 499.0;
const LOAD_MIN: f64  = 5_001.0;

pub struct TickGenerator {
    lmps:       Vec<f64>,
    idx:        usize,
    base_lmp:   f64,
    volatility: f64,
    lmp:        f64,
    rng:        rand::rngs::SmallRng,
    dist:       Normal<f64>,
}

impl TickGenerator {
    pub fn new(base_lmp: f64, volatility: f64, seed: u32) -> Self {
        Self {
            lmps:       vec![],
            idx:        0,
            base_lmp,
            volatility,
            lmp:        base_lmp,
            rng:        rand::rngs::SmallRng::seed_from_u64(seed as u64),
            dist:       Normal::new(0.0, 1.0).unwrap(),
        }
    }

    pub fn next(&mut self, out_lmp: &mut f64, out_load_mw: &mut f64) {
        if !self.lmps.is_empty() {
            *out_lmp = self.lmps[self.idx % self.lmps.len()];
            self.idx += 1;
        } else {
            self.lmp += THETA * (self.base_lmp - self.lmp)
                + self.volatility * self.dist.sample(&mut self.rng);
            self.lmp = self.lmp.clamp(LMP_MIN, LMP_MAX);
            *out_lmp = self.lmp;
        }
        let load = BASE_LOAD + LOAD_VOL * self.dist.sample(&mut self.rng);
        *out_load_mw = load.max(LOAD_MIN);
    }
}

fn parse_ercot_json(body: &str) -> Option<Vec<f64>> {
    let v: serde_json::Value = serde_json::from_str(body).ok()?;
    let rows = v.get("data")?.as_array()?;
    let prices: Vec<f64> = rows.iter()
        .filter_map(|row| row.get(4)?.as_f64())
        .collect();
    if prices.is_empty() { None } else { Some(prices) }
}

fn fetch_ercot_lmps(settlement_point: &str, date: &str) -> Option<Vec<f64>> {
    let url = format!(
        "https://api.ercot.com/api/public-reports/np4-190-cd/dam_stlmnt_pnt_prices\
         ?deliveryDateFrom={date}&deliveryDateTo={date}\
         &settlementPoint={settlement_point}&size=96"
    );
    let body = ureq::get(&url)
        .call()
        .ok()?
        .into_string()
        .ok()?;
    parse_ercot_json(&body)
}

// --- extern "C" ABI — same symbols as the former C++ libmarketdata.so ---

#[no_mangle]
pub extern "C" fn tick_generator_create(
    base_lmp: f64,
    volatility: f64,
    seed: u32,
) -> *mut TickGenerator {
    let node = std::env::var("NODE_NAME")
        .unwrap_or_else(|_| "HB_NORTH".to_string());
    let date = std::env::var("ERCOT_REPLAY_DATE")
        .unwrap_or_else(|_| "2024-01-15".to_string());

    let lmps = match fetch_ercot_lmps(&node, &date) {
        Some(v) => {
            eprintln!("tick-engine: loaded {} ERCOT LMPs for {} ({})", v.len(), node, date);
            v
        }
        None => {
            eprintln!("tick-engine: ERCOT fetch failed for {}, using OU fallback", node);
            vec![]
        }
    };

    Box::into_raw(Box::new(TickGenerator {
        lmps,
        idx:        0,
        base_lmp,
        volatility,
        lmp:        base_lmp,
        rng:        rand::rngs::SmallRng::seed_from_u64(seed as u64),
        dist:       Normal::new(0.0, 1.0).unwrap(),
    }))
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
    if gen.is_null() || out_lmp.is_null() || out_load_mw.is_null() {
        return;
    }
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

    #[test]
    fn parse_lmps_extracts_prices() {
        let json = r#"{"data":[["2024-01-15",1,1,"HB_NORTH",23.45],["2024-01-15",1,2,"HB_NORTH",24.10],["2024-01-15",1,3,"HB_NORTH",22.80]]}"#;
        let result = parse_ercot_json(json);
        assert_eq!(result, Some(vec![23.45, 24.10, 22.80]));
    }

    #[test]
    fn parse_lmps_returns_none_on_bad_json() {
        assert_eq!(parse_ercot_json("not json"), None);
    }

    #[test]
    fn replay_cycles_buffer() {
        let mut g = TickGenerator {
            lmps:       vec![10.0, 20.0, 30.0],
            idx:        0,
            base_lmp:   45.0,
            volatility: 5.0,
            lmp:        45.0,
            rng:        rand::rngs::SmallRng::seed_from_u64(42),
            dist:       Normal::new(0.0, 1.0).unwrap(),
        };
        let (mut lmp, mut load) = (0.0f64, 0.0f64);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 10.0);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 20.0);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 30.0);
        g.next(&mut lmp, &mut load); assert_eq!(lmp, 10.0, "should wrap around");
    }

    #[test]
    fn replay_uses_ou_when_empty() {
        let mut g = TickGenerator {
            lmps:       vec![],
            idx:        0,
            base_lmp:   45.0,
            volatility: 5.0,
            lmp:        45.0,
            rng:        rand::rngs::SmallRng::seed_from_u64(42),
            dist:       Normal::new(0.0, 1.0).unwrap(),
        };
        for _ in 0..100 {
            let (mut lmp, mut load) = (0.0f64, 0.0f64);
            g.next(&mut lmp, &mut load);
            assert!(lmp >= 1.0 && lmp <= 499.0, "lmp={lmp} out of [1,499]");
            assert!(load > 5000.0, "load={load} not > 5000");
        }
    }
}
