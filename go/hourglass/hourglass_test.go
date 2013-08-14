package hourglass

import (
	"sort"
	"testing"
	"time"
)

// Test type. true - use system time; false - use sandbox time.
var types = []bool{false, true}

func TestAdvance(t *testing.T) {
	SetRealTime(false)
	t1 := Now()
	d := time.Second * 11
	Advance(t, d)
	t2 := Now()
	dt := t2.Sub(t1)
	if dt != d {
		t.Fatalf("TestAdvanceHourglass: want=%d have=%d", d, dt)
	}
}

func TestSleep(t *testing.T) {
	SetRealTime(false)
	const delay = 100 * time.Millisecond
	go func() {
		// pause for a while to make sure main thread is waiting
		SleepSys(t, delay/2)
		Advance(t, delay)
	}()
	start := Now()
	Sleep(delay)
	duration := Now().Sub(start)
	if duration != delay {
		t.Fatalf("Sleep(%s) slept for %s", delay, duration)
	}
}

func TestAfterFunc(t *testing.T) {
	for _, ttype := range types {
		SetRealTime(ttype)
		i := 10
		c := make(chan bool)
		var f func()
		f = func() {
			i--
			if i >= 0 {
				AfterFunc(0, f)
				if ttype {
					Sleep(time.Second)
				} else {
					Advance(t, time.Second)
				}
			} else {
				c <- true
			}
		}
		AfterFunc(0, f)
		<-c
	}
}

func TestAfter(t *testing.T) {
	for _, ttype := range types {
		SetRealTime(ttype)
		const delay = 100 * time.Millisecond
		start := Now()
		c := After(delay)
		if !ttype {
			Advance(t, delay)
		}
		end := <-c
		if duration := Now().Sub(start); duration < delay {
			t.Fatalf("After(%s) slept for %s", delay, duration)
		}
		if min := start.Add(delay); end.Before(min) {
			t.Fatalf("After(%s) expect >= %s, got %s", delay, min, end)
		}
	}
}

func TestAfterTick(t *testing.T) {
	const Count = 10
	Delta := 100 * time.Millisecond
	if testing.Short() {
		Delta = 10 * time.Millisecond
	}
	for _, ttype := range types {
		SetRealTime(ttype)
		t0 := Now()
		for i := 0; i < Count; i++ {
			c := After(Delta)
			if !ttype {
				Advance(t, Delta)
			}
			<-c
		}
		t1 := Now()
		d := t1.Sub(t0)
		target := Delta * Count
		if d < target*9/10 {
			t.Fatalf("%d ticks of %s too fast: took %s, expected %s", Count, Delta, d, target)
		}
		if !testing.Short() && d > target*30/10 {
			t.Fatalf("%d ticks of %s too slow: took %s, expected %s", Count, Delta, d, target)
		}
	}
}

func TestAfterStop(t *testing.T) {
	for _, ttype := range types {
		SetRealTime(ttype)
		AfterFunc(100*time.Millisecond, func() {})
		t0 := NewTimer(50 * time.Millisecond)
		c1 := make(chan bool, 1)
		t1 := AfterFunc(150*time.Millisecond, func() { c1 <- true })
		c2 := After(200 * time.Millisecond)
		if !t0.Stop() {
			t.Fatalf("failed to stop event 0")
		}
		if !t1.Stop() {
			t.Fatalf("failed to stop event 1")
		}
		if !ttype {
			Advance(t, 250*time.Millisecond)
		}
		<-c2
		select {
		case <-t0.C:
			t.Fatalf("event 0 was not stopped")
		case <-c1:
			t.Fatalf("event 1 was not stopped")
		default:
		}
		if t1.Stop() {
			t.Fatalf("Stop returned true twice")
		}
	}
}

var slots = []int{5, 3, 6, 6, 6, 1, 1, 2, 7, 9, 4, 8, 0}

type afterResult struct {
	slot int
	t    time.Time
}

func await(slot int, result chan<- afterResult, ac <-chan time.Time) {
	result <- afterResult{slot, <-ac}
}
func TestAfterQueuing(t *testing.T) {
	Delta := 100 * time.Millisecond
	if testing.Short() {
		Delta = 20 * time.Millisecond
	}
	for _, ttype := range types {
		SetRealTime(ttype)
		// make the result channel buffered because we don't want
		// to depend on channel queueing semantics that might
		// possibly change in the future.
		result := make(chan afterResult, len(slots))

		t0 := Now()
		for _, slot := range slots {
			go await(slot, result, After(time.Duration(slot)*Delta))
		}

		if !ttype {
			// advance time in interval of duration Delay
			// to receive time in expected order in await().
			// if advance time in one large step, it cannot control the order that
			// await()s are being called, and hence the results are out of order.
			// cannot use AfterFunc() with customized func as well,
			// because per Go time implementation, func passed in AfterFunc() are called
			// in separate goroutine ("go func()")
			max := slots[0]
			for _, value := range slots {
				if value > max {
					max = value
				}
			}
			for i := 0; i <= max; i++ {
				SleepSys(t, Delta)
				Advance(t, Delta)
			}
		}

		sort.Ints(slots)
		for _, slot := range slots {
			r := <-result
			if r.slot != slot {
				t.Fatalf("after slot %d, expected %d", r.slot, slot)
			}
			dt := r.t.Sub(t0)
			target := time.Duration(slot) * Delta
			if dt < target-Delta/2 || dt > target+Delta*10 {
				t.Fatalf("After(%s) arrived at %s, expected [%s,%s]", target, dt, target-Delta/2, target+Delta*10)
			}
		}
	}
}

func TestReset(t *testing.T) {
	const delay = 100 * time.Millisecond
	for _, ttype := range types {
		SetRealTime(ttype)
		t0 := NewTimer(2 * delay)
		if ttype {
			Sleep(delay)
		} else {
			Advance(t, delay)
		}
		if t0.Reset(3*delay) != true {
			t.Fatalf("resetting unfired timer returned false")
		}
		if ttype {
			Sleep(2 * delay)
		} else {
			Advance(t, 2*delay)
		}
		select {
		case <-t0.C:
			t.Fatalf("time fired early")
		default:
		}
		if ttype {
			Sleep(2 * delay)
		} else {
			Advance(t, 2*delay)
		}
		select {
		case <-t0.C:
		default:
			t.Fatalf("reset timer did not fire")
		}
		if t0.Reset(50*time.Millisecond) != false {
			t.Fatalf("resetting expired timer returned true")
		}
	}
}

func TestOverflowSleep(t *testing.T) {
	const timeout = 25 * time.Millisecond
	const big = time.Duration(int64(1<<63 - 1))
	for _, ttype := range types {
		SetRealTime(ttype)
		c1 := After(big)
		c2 := After(timeout)
		if !ttype {
			Advance(t, timeout)
		}
		select {
		case <-c1:
			t.Fatalf("big timeout fired")
		case <-c2:
			// OK
		}
		const neg = time.Duration(-1 << 63)
		select {
		case <-After(neg):
			// OK
		case <-After(timeout):
			t.Fatalf("negative timeout didn't fire")
		}
	}
}

// tick_test.go

func TestTicker(t *testing.T) {
	for _, ttype := range types {
		SetRealTime(ttype)
		const Count = 10
		Delta := 100 * time.Millisecond
		ticker := NewTicker(Delta)
		if ttype == false {
			go func() {
				for i := 0; i < Count; i++ {
					// sleep so will not drop ticks
					SleepSys(t, Delta)
					Advance(t, Delta)
				}
			}()
		}
		t0 := Now()
		for i := 0; i < Count; i++ {
			<-ticker.C
		}
		ticker.Stop()
		t1 := Now()
		dt := t1.Sub(t0)
		target := Delta * Count
		slop := target * 2 / 10
		if dt < target-slop || (!testing.Short() && dt > target+slop) {
			t.Fatalf("%d %s ticks took %s, expected [%s,%s]", Count, Delta, dt, target-slop, target+slop)
		}
		// Now test that the ticker stopped
		if ttype {
			Sleep(2 * Delta)
		} else {
			Advance(t, 2*Delta)
		}
		select {
		case <-ticker.C:
			t.Fatal("Ticker did not shut down")
		default:
			// ok
		}
	}
}

func TestTeardown(t *testing.T) {
	for _, ttype := range types {
		SetRealTime(ttype)
		Delta := 100 * time.Millisecond
		if testing.Short() {
			Delta = 20 * time.Millisecond
		}
		for i := 0; i < 3; i++ {
			ticker := NewTicker(Delta)
			if !ttype {
				go func() {
					Advance(t, Delta)
				}()
			}
			<-ticker.C
			ticker.Stop()
		}
	}
}

var t time.Time
var u int64

func BenchmarkNowTime(b *testing.B) {
	for i := 0; i < b.N; i++ {
		t = time.Now()
	}
}

func BenchmarkNowUnixNanoTime(b *testing.B) {
	for i := 0; i < b.N; i++ {
		u = time.Now().UnixNano()
	}
}

func BenchmarkNowHourglassSys(b *testing.B) {
	SetRealTime(true)
	for i := 0; i < b.N; i++ {
		t = Now()
	}
}

func BenchmarkNowUnixNanoHourglassSys(b *testing.B) {
	SetRealTime(true)
	for i := 0; i < b.N; i++ {
		u = Now().UnixNano()
	}
}
