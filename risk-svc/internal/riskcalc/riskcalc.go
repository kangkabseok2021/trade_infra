package riskcalc

/*
#cgo CFLAGS: -I${SRCDIR}/../../include
#cgo darwin LDFLAGS: -L${SRCDIR}/../../build -lriskcalc -Wl,-rpath,${SRCDIR}/../../build
#cgo linux  LDFLAGS: -L${SRCDIR}/../../build -lriskcalc -Wl,-rpath,${SRCDIR}/../../build
#include "riskcalc.h"
*/
import "C"

func CalcMtmPnl(netMW, avgFillPrice, currentLMP float64) float64 {
	return float64(C.calc_mtm_pnl(C.double(netMW), C.double(avgFillPrice), C.double(currentLMP)))
}

func CalcNetExposure(netMW, currentLMP float64) float64 {
	return float64(C.calc_net_exposure(C.double(netMW), C.double(currentLMP)))
}

func CheckLimitBreach(netExposureMW, positionLimitMW float64) bool {
	return C.check_limit_breach(C.double(netExposureMW), C.double(positionLimitMW)) == 1
}
