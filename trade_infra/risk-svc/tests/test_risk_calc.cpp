#include <gtest/gtest.h>
#include "riskcalc.h"

TEST(RiskCalcTest, MtmPnlLongPositionProfitable) {
    // Long 10 MW, avg fill $40, current $45 → profit = 10 * 5 = $50
    EXPECT_DOUBLE_EQ(calc_mtm_pnl(10.0, 40.0, 45.0), 50.0);
}

TEST(RiskCalcTest, MtmPnlLongPositionLoss) {
    // Long 10 MW, avg fill $40, current $35 → loss = 10 * -5 = -$50
    EXPECT_DOUBLE_EQ(calc_mtm_pnl(10.0, 40.0, 35.0), -50.0);
}

TEST(RiskCalcTest, MtmPnlShortPosition) {
    // Short -10 MW, avg fill $40, current $35 → profit = -10 * -5 = $50
    EXPECT_DOUBLE_EQ(calc_mtm_pnl(-10.0, 40.0, 35.0), 50.0);
}

TEST(RiskCalcTest, MtmPnlFlatPosition) {
    EXPECT_DOUBLE_EQ(calc_mtm_pnl(0.0, 40.0, 45.0), 0.0);
}

TEST(RiskCalcTest, NetExposureLong) {
    // 10 MW long → 10 MW net exposure (LMP does not affect MW position)
    EXPECT_DOUBLE_EQ(calc_net_exposure(10.0, 45.0), 10.0);
}

TEST(RiskCalcTest, NetExposureShort) {
    // -10 MW short → 10 MW net exposure (absolute)
    EXPECT_DOUBLE_EQ(calc_net_exposure(-10.0, 45.0), 10.0);
}

TEST(RiskCalcTest, LimitBreachExceeded) {
    EXPECT_EQ(check_limit_breach(15.0, 10.0), 1);
}

TEST(RiskCalcTest, LimitBreachNotExceeded) {
    EXPECT_EQ(check_limit_breach(8.0, 10.0), 0);
}

TEST(RiskCalcTest, LimitBreachAtExactLimit) {
    EXPECT_EQ(check_limit_breach(10.0, 10.0), 0);
}
