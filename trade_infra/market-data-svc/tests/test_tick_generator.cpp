#include <gtest/gtest.h>
#include "marketdata.h"

TEST(TickGeneratorTest, CreateAndDestroy) {
    TickGenerator* gen = tick_generator_create(45.0, 5.0, 42);
    ASSERT_NE(gen, nullptr);
    tick_generator_destroy(gen);
}

TEST(TickGeneratorTest, LMPIsPositive) {
    TickGenerator* gen = tick_generator_create(45.0, 5.0, 42);
    double lmp, load;
    tick_generator_next(gen, &lmp, &load);
    EXPECT_GT(lmp, 0.0);
    tick_generator_destroy(gen);
}

TEST(TickGeneratorTest, LMPStaysInRange) {
    TickGenerator* gen = tick_generator_create(45.0, 5.0, 42);
    double lmp, load;
    for (int i = 0; i < 1000; ++i) {
        tick_generator_next(gen, &lmp, &load);
        EXPECT_GT(lmp, 0.0) << "negative at tick " << i;
        EXPECT_LT(lmp, 500.0) << "too high at tick " << i;
    }
    tick_generator_destroy(gen);
}

TEST(TickGeneratorTest, LoadMWInRange) {
    TickGenerator* gen = tick_generator_create(45.0, 5.0, 42);
    double lmp, load;
    tick_generator_next(gen, &lmp, &load);
    EXPECT_GT(load, 5000.0);
    EXPECT_LT(load, 30000.0);
    tick_generator_destroy(gen);
}

TEST(TickGeneratorTest, DeterministicWithSameSeed) {
    TickGenerator* g1 = tick_generator_create(45.0, 5.0, 42);
    TickGenerator* g2 = tick_generator_create(45.0, 5.0, 42);
    double l1, l2, d1, d2;
    for (int i = 0; i < 20; ++i) {
        tick_generator_next(g1, &l1, &d1);
        tick_generator_next(g2, &l2, &d2);
        EXPECT_DOUBLE_EQ(l1, l2) << "diverged at tick " << i;
    }
    tick_generator_destroy(g1);
    tick_generator_destroy(g2);
}
