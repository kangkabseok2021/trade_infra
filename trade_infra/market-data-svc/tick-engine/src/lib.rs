// Implementation will be added in Task 3.

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
