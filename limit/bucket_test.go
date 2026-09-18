// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package limit

import (
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestBucketUpsert(t *testing.T) {
	testCases := []struct {
		name           string
		bucketCapacity int
		alerts         []model.Alert
		alertTimings   []time.Time // When each alert is added relative to now
		expectedResult []bool      // Expected return value for each Add() call
		description    string
	}{
		{
			name:           "Bucket with zero capacity should reject all alerts",
			bucketCapacity: 0,
			alerts: []model.Alert{
				{Labels: model.LabelSet{"alertname": "Alert1", "instance": "server1"}, EndsAt: time.Now().Add(1 * time.Hour)},
				{Labels: model.LabelSet{"alertname": "Alert2", "instance": "server2"}, EndsAt: time.Now().Add(1 * time.Hour)},
				{Labels: model.LabelSet{"alertname": "Alert3", "instance": "server3"}, EndsAt: time.Now().Add(1 * time.Hour)},
			},
			alertTimings:   []time.Time{time.Now(), time.Now(), time.Now()},
			expectedResult: []bool{false, false, false}, // All should be rejected
			description:    "Adding 3 alerts to a bucket with capacity 0 should fail",
		},
		{
			name:           "Empty bucket should add items while not full",
			bucketCapacity: 3,
			alerts: []model.Alert{
				{Labels: model.LabelSet{"alertname": "Alert1", "instance": "server1"}, EndsAt: time.Now().Add(1 * time.Hour)},
				{Labels: model.LabelSet{"alertname": "Alert2", "instance": "server2"}, EndsAt: time.Now().Add(1 * time.Hour)},
				{Labels: model.LabelSet{"alertname": "Alert3", "instance": "server3"}, EndsAt: time.Now().Add(1 * time.Hour)},
			},
			alertTimings:   []time.Time{time.Now(), time.Now(), time.Now()},
			expectedResult: []bool{true, true, true}, // All should be added successfully
			description:    "Adding 3 alerts to a bucket with capacity 3 should succeed",
		},
		{
			name:           "Full bucket must not add items if old items are not expired yet",
			bucketCapacity: 2,
			alerts: []model.Alert{
				{Labels: model.LabelSet{"alertname": "Alert1", "instance": "server1"}, EndsAt: time.Now().Add(1 * time.Hour)},
				{Labels: model.LabelSet{"alertname": "Alert2", "instance": "server2"}, EndsAt: time.Now().Add(1 * time.Hour)},
				{Labels: model.LabelSet{"alertname": "Alert3", "instance": "server3"}, EndsAt: time.Now().Add(1 * time.Hour)},
			},
			alertTimings:   []time.Time{time.Now(), time.Now(), time.Now()},
			expectedResult: []bool{true, true, false}, // First two succeed, third fails
			description:    "Adding third alert to full bucket with non-expired items should fail",
		},
		{
			name:           "Full bucket must add items if old items are expired",
			bucketCapacity: 2,
			alerts: []model.Alert{
				{Labels: model.LabelSet{"alertname": "Alert1", "instance": "server1"}, EndsAt: time.Now().Add(-1 * time.Hour)},    // Expired 1 hour ago
				{Labels: model.LabelSet{"alertname": "Alert2", "instance": "server2"}, EndsAt: time.Now().Add(-30 * time.Minute)}, // Expired 30 minutes ago
				{Labels: model.LabelSet{"alertname": "Alert3", "instance": "server3"}, EndsAt: time.Now().Add(1 * time.Hour)},     // Will expire in 1 hour
			},
			alertTimings:   []time.Time{time.Now(), time.Now(), time.Now()},
			expectedResult: []bool{true, true, true}, // All should succeed because older items get evicted
			description:    "Adding new alerts when bucket is full but oldest items are expired should succeed",
		},
		{
			name:           "Update existing alert in bucket should not increase size",
			bucketCapacity: 2,
			alerts: []model.Alert{
				{Labels: model.LabelSet{"alertname": "Alert1", "instance": "server1"}, EndsAt: time.Now().Add(1 * time.Hour)},
				{Labels: model.LabelSet{"alertname": "Alert1", "instance": "server1"}, EndsAt: time.Now().Add(2 * time.Hour)}, // Same fingerprint, different EndsAt
				{Labels: model.LabelSet{"alertname": "Alert2", "instance": "server2"}, EndsAt: time.Now().Add(1 * time.Hour)},
			},
			alertTimings:   []time.Time{time.Now(), time.Now(), time.Now()},
			expectedResult: []bool{true, true, true}, // All should succeed - second is an update, not a new entry
			description:    "Updating existing alert should not consume additional bucket space",
		},
		{
			name:           "Mixed scenario with expiration and updates",
			bucketCapacity: 2,
			alerts: []model.Alert{
				{Labels: model.LabelSet{"alertname": "Alert1", "instance": "server1"}, EndsAt: time.Now().Add(-1 * time.Hour)}, // Expired
				{Labels: model.LabelSet{"alertname": "Alert2", "instance": "server2"}, EndsAt: time.Now().Add(1 * time.Hour)},  // Active
				{Labels: model.LabelSet{"alertname": "Alert1", "instance": "server1"}, EndsAt: time.Now().Add(2 * time.Hour)},  // Update of first alert
				{Labels: model.LabelSet{"alertname": "Alert3", "instance": "server3"}, EndsAt: time.Now().Add(1 * time.Hour)},  // New alert, bucket full but Alert2 not expired
			},
			alertTimings:   []time.Time{time.Now(), time.Now(), time.Now(), time.Now()},
			expectedResult: []bool{true, true, true, false}, // Last one should fail because bucket is full with non-expired items
			description:    "Complex scenario with expiration, updates, and eviction should work correctly",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bucket := NewBucket[model.Fingerprint](tc.bucketCapacity)

			for i, alert := range tc.alerts {
				result := bucket.Upsert(alert.Fingerprint(), alert.EndsAt)
				require.Equal(t, tc.expectedResult[i], result,
					"Alert %d: expected %v, got %v. %s", i+1, tc.expectedResult[i], result, tc.description)
			}
		})
	}
}

func TestBucketAddConcurrency(t *testing.T) {
	bucket := NewBucket[model.Fingerprint](2)

	// Test that concurrent access to bucket is safe
	alert1 := model.Alert{Labels: model.LabelSet{"alertname": "Alert1", "instance": "server1"}, EndsAt: time.Now().Add(1 * time.Hour)}
	alert2 := model.Alert{Labels: model.LabelSet{"alertname": "Alert2", "instance": "server2"}, EndsAt: time.Now().Add(1 * time.Hour)}

	done := make(chan bool, 2)

	// Add alerts concurrently
	go func() {
		bucket.Upsert(alert1.Fingerprint(), alert1.EndsAt)
		done <- true
	}()

	go func() {
		bucket.Upsert(alert2.Fingerprint(), alert2.EndsAt)
		done <- true
	}()

	// Wait for both goroutines to complete
	<-done
	<-done

	// Verify that both alerts were added (bucket should contain 2 items)
	require.Len(t, bucket.index, 2, "Expected 2 alerts in bucket after concurrent adds")
	require.Len(t, bucket.items, 2, "Expected 2 items in bucket map after concurrent adds")
}

func TestBucketAddExpiredEviction(t *testing.T) {
	bucket := NewBucket[model.Fingerprint](2)

	// Add two alerts that are already expired
	expiredAlert1 := model.Alert{
		Labels: model.LabelSet{"alertname": "ExpiredAlert1", "instance": "server1"},
		EndsAt: time.Now().Add(-1 * time.Hour),
	}
	expiredFingerprint1 := expiredAlert1.Fingerprint()
	expiredAlert2 := model.Alert{
		Labels: model.LabelSet{"alertname": "ExpiredAlert2", "instance": "server2"},
		EndsAt: time.Now().Add(-30 * time.Minute),
	}
	expiredFingerprint2 := expiredAlert2.Fingerprint()

	// Fill the bucket with expired alerts
	result1 := bucket.Upsert(expiredFingerprint1, expiredAlert1.EndsAt)
	require.True(t, result1, "First expired alert should be added successfully")

	result2 := bucket.Upsert(expiredFingerprint2, expiredAlert2.EndsAt)
	require.True(t, result2, "Second expired alert should be added successfully")

	// Now add a fresh alert - it should evict the first expired alert
	freshAlert := model.Alert{
		Labels: model.LabelSet{"alertname": "FreshAlert", "instance": "server3"},
		EndsAt: time.Now().Add(1 * time.Hour),
	}
	freshFingerprint := freshAlert.Fingerprint()

	result3 := bucket.Upsert(freshFingerprint, freshAlert.EndsAt)
	require.True(t, result3, "Fresh alert should be added successfully, evicting expired alert")

	// Verify the bucket state
	require.Len(t, bucket.index, 2, "Bucket should still contain 2 items after eviction")
	require.Len(t, bucket.items, 2, "Bucket map should still contain 2 items after eviction")

	// The fresh alert should be in the bucket
	_, exists := bucket.index[freshFingerprint]
	require.True(t, exists, "Fresh alert should exist in bucket after eviction")

	// The first expired alert should have been evicted
	_, exists = bucket.index[expiredFingerprint1]
	require.False(t, exists, "First expired alert should have been evicted from bucket, fingerprint: %d", expiredFingerprint1)
}

func TestBucketAddEdgeCases(t *testing.T) {
	t.Run("Single capacity bucket with replacement", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](1)

		// Add expired alert
		expiredAlert := model.Alert{Labels: model.LabelSet{"alertname": "Expired"}, EndsAt: time.Now().Add(-1 * time.Hour)}
		result1 := bucket.Upsert(expiredAlert.Fingerprint(), expiredAlert.EndsAt)
		require.True(t, result1, "Adding expired alert to single-capacity bucket should succeed")

		// Add fresh alert (should replace expired one)
		freshAlert := model.Alert{Labels: model.LabelSet{"alertname": "Fresh"}, EndsAt: time.Now().Add(1 * time.Hour)}
		result2 := bucket.Upsert(freshAlert.Fingerprint(), freshAlert.EndsAt)
		require.True(t, result2, "Adding fresh alert should succeed by replacing expired one")

		// Verify only the fresh alert remains
		require.Len(t, bucket.index, 1, "Bucket should contain exactly 1 item")
		freshFingerprint := freshAlert.Fingerprint()
		_, exists := bucket.index[freshFingerprint]
		require.True(t, exists, "Fresh alert should exist in bucket")
	})

	t.Run("Alert with same fingerprint but different EndsAt", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](2)

		// Add initial alert
		originalTime := time.Now().Add(1 * time.Hour)
		alert1 := model.Alert{Labels: model.LabelSet{"alertname": "Test"}, EndsAt: originalTime}
		result1 := bucket.Upsert(alert1.Fingerprint(), alert1.EndsAt)
		require.True(t, result1, "Initial alert should be added successfully")

		// Add same alert with different EndsAt (should update, not add new)
		updatedTime := time.Now().Add(2 * time.Hour)
		alert2 := model.Alert{Labels: model.LabelSet{"alertname": "Test"}, EndsAt: updatedTime}
		result2 := bucket.Upsert(alert2.Fingerprint(), alert2.EndsAt)
		require.True(t, result2, "Updated alert should not fill bucket")

		// Verify bucket still has only one entry with updated time
		require.Len(t, bucket.index, 1, "Bucket should contain exactly 1 item after update")
		fingerprint := alert1.Fingerprint()
		storedTime, exists := bucket.index[fingerprint]
		require.True(t, exists, "Alert should exist in bucket")
		require.Equal(t, updatedTime, storedTime.priority, "Alert should have updated EndsAt time")
	})
}

func TestBucketRemove(t *testing.T) {
	t.Run("Remove present fingerprint returns true and frees its slot", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](2)
		alert := model.Alert{Labels: model.LabelSet{"alertname": "Alert1"}, EndsAt: time.Now().Add(1 * time.Hour)}

		require.True(t, bucket.Upsert(alert.Fingerprint(), alert.EndsAt), "alert should be added")

		require.True(t, bucket.Remove(alert.Fingerprint()), "removing a present fingerprint should return true")
		require.Empty(t, bucket.index, "index should be empty after removal")
		require.Zero(t, bucket.items.Len(), "heap should be empty after removal")
	})

	t.Run("Remove absent fingerprint returns false", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](2)
		present := model.Alert{Labels: model.LabelSet{"alertname": "Present"}, EndsAt: time.Now().Add(1 * time.Hour)}
		bucket.Upsert(present.Fingerprint(), present.EndsAt)

		absent := model.Alert{Labels: model.LabelSet{"alertname": "Absent"}}
		require.False(t, bucket.Remove(absent.Fingerprint()), "removing an absent fingerprint should return false")
		require.Len(t, bucket.index, 1, "present item should remain")
		require.Equal(t, 1, bucket.items.Len(), "present item should remain in heap")
	})

	t.Run("Remove from empty bucket returns false", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](2)
		alert := model.Alert{Labels: model.LabelSet{"alertname": "Alert1"}}
		require.False(t, bucket.Remove(alert.Fingerprint()), "removing from an empty bucket should return false")
	})

	t.Run("Removing twice returns false the second time", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](2)
		alert := model.Alert{Labels: model.LabelSet{"alertname": "Alert1"}, EndsAt: time.Now().Add(1 * time.Hour)}
		bucket.Upsert(alert.Fingerprint(), alert.EndsAt)

		require.True(t, bucket.Remove(alert.Fingerprint()), "first removal should return true")
		require.False(t, bucket.Remove(alert.Fingerprint()), "second removal should return false")
	})

	t.Run("Remove frees a slot in a full bucket so a new alert is admitted", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](2)
		a := model.Alert{Labels: model.LabelSet{"alertname": "A"}, EndsAt: time.Now().Add(1 * time.Hour)}
		b := model.Alert{Labels: model.LabelSet{"alertname": "B"}, EndsAt: time.Now().Add(1 * time.Hour)}
		c := model.Alert{Labels: model.LabelSet{"alertname": "C"}, EndsAt: time.Now().Add(1 * time.Hour)}

		require.True(t, bucket.Upsert(a.Fingerprint(), a.EndsAt), "A should be added")
		require.True(t, bucket.Upsert(b.Fingerprint(), b.EndsAt), "B should be added")
		require.False(t, bucket.Upsert(c.Fingerprint(), c.EndsAt), "C should be rejected while bucket is full of active items")

		require.True(t, bucket.Remove(a.Fingerprint()), "A should be removed")
		require.True(t, bucket.Upsert(c.Fingerprint(), c.EndsAt), "C should be admitted after A freed a slot")

		_, hasA := bucket.index[a.Fingerprint()]
		_, hasC := bucket.index[c.Fingerprint()]
		require.False(t, hasA, "A should no longer be present")
		require.True(t, hasC, "C should now be present")
		require.Len(t, bucket.index, 2, "bucket should hold B and C")
	})

	t.Run("Remove keeps heap eviction order intact", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](3)
		oldest := model.Alert{Labels: model.LabelSet{"alertname": "Oldest"}, EndsAt: time.Now().Add(-2 * time.Hour)}
		middle := model.Alert{Labels: model.LabelSet{"alertname": "Middle"}, EndsAt: time.Now().Add(-1 * time.Hour)}
		newest := model.Alert{Labels: model.LabelSet{"alertname": "Newest"}, EndsAt: time.Now().Add(1 * time.Hour)}

		bucket.Upsert(oldest.Fingerprint(), oldest.EndsAt)
		bucket.Upsert(middle.Fingerprint(), middle.EndsAt)
		bucket.Upsert(newest.Fingerprint(), newest.EndsAt)

		// Remove the current heap root (oldest); the next-oldest expired item
		// must then sit at the root and be the one evicted when full.
		require.True(t, bucket.Remove(oldest.Fingerprint()), "oldest should be removed")
		require.Equal(t, middle.Fingerprint(), bucket.items[0].value, "middle should be the new heap root")

		// Bucket has room for one more; fill it, then force an eviction.
		other := model.Alert{Labels: model.LabelSet{"alertname": "Other"}, EndsAt: time.Now().Add(2 * time.Hour)}
		bucket.Upsert(other.Fingerprint(), other.EndsAt)

		evictor := model.Alert{Labels: model.LabelSet{"alertname": "Evictor"}, EndsAt: time.Now().Add(3 * time.Hour)}
		require.True(t, bucket.Upsert(evictor.Fingerprint(), evictor.EndsAt), "evictor should replace the expired middle item")

		_, hasMiddle := bucket.index[middle.Fingerprint()]
		require.False(t, hasMiddle, "expired middle item should have been evicted")
		require.Len(t, bucket.index, 3, "index and heap should stay consistent")
		require.Equal(t, 3, bucket.items.Len(), "index and heap should stay consistent")
	})
}

func TestBucketRemoveConcurrency(t *testing.T) {
	bucket := NewBucket[model.Fingerprint](2)
	alert1 := model.Alert{Labels: model.LabelSet{"alertname": "Alert1"}, EndsAt: time.Now().Add(1 * time.Hour)}
	alert2 := model.Alert{Labels: model.LabelSet{"alertname": "Alert2"}, EndsAt: time.Now().Add(1 * time.Hour)}
	bucket.Upsert(alert1.Fingerprint(), alert1.EndsAt)
	bucket.Upsert(alert2.Fingerprint(), alert2.EndsAt)

	done := make(chan bool, 2)
	go func() {
		bucket.Remove(alert1.Fingerprint())
		done <- true
	}()
	go func() {
		bucket.Remove(alert2.Fingerprint())
		done <- true
	}()
	<-done
	<-done

	require.Empty(t, bucket.index, "both alerts should be removed after concurrent removes")
	require.Zero(t, bucket.items.Len(), "heap should be empty after concurrent removes")
}

// Benchmark tests for Bucket.Upsert() performance.
func BenchmarkBucketUpsert(b *testing.B) {
	b.Run("EmptyBucket", func(b *testing.B) {
		bucket := NewBucket[model.Fingerprint](1000)
		alert := model.Alert{
			Labels: model.LabelSet{"alertname": "TestAlert", "instance": "server1"},
			EndsAt: time.Now().Add(1 * time.Hour),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bucket.Upsert(alert.Fingerprint(), alert.EndsAt)
		}
	})

	b.Run("AddToFullBucketWithExpiredItems", func(b *testing.B) {
		bucketSize := 100
		bucket := NewBucket[model.Fingerprint](bucketSize)

		// Fill bucket with expired alerts
		for i := range bucketSize {
			expiredAlert := model.Alert{
				Labels: model.LabelSet{"alertname": model.LabelValue("ExpiredAlert" + string(rune(i))), "instance": "server1"},
				EndsAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
			}
			bucket.Upsert(expiredAlert.Fingerprint(), expiredAlert.EndsAt)
		}

		newAlert := model.Alert{
			Labels: model.LabelSet{"alertname": "NewAlert", "instance": "server2"},
			EndsAt: time.Now().Add(1 * time.Hour),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bucket.Upsert(newAlert.Fingerprint(), newAlert.EndsAt)
		}
	})

	b.Run("AddToFullBucketWithActiveItems", func(b *testing.B) {
		bucketSize := 100
		bucket := NewBucket[model.Fingerprint](bucketSize)

		// Fill bucket with active alerts
		for i := range bucketSize {
			activeAlert := model.Alert{
				Labels: model.LabelSet{"alertname": model.LabelValue("ActiveAlert" + string(rune(i))), "instance": "server1"},
				EndsAt: time.Now().Add(1 * time.Hour), // Active for 1 hour
			}
			bucket.Upsert(activeAlert.Fingerprint(), activeAlert.EndsAt)
		}

		newAlert := model.Alert{
			Labels: model.LabelSet{"alertname": "NewAlert", "instance": "server2"},
			EndsAt: time.Now().Add(1 * time.Hour),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bucket.Upsert(newAlert.Fingerprint(), newAlert.EndsAt)
		}
	})

	b.Run("UpdateExistingItem", func(b *testing.B) {
		bucket := NewBucket[model.Fingerprint](100)

		// Add initial alert
		alert := model.Alert{
			Labels: model.LabelSet{"alertname": "TestAlert", "instance": "server1"},
			EndsAt: time.Now().Add(1 * time.Hour),
		}
		bucket.Upsert(alert.Fingerprint(), alert.EndsAt)

		// Create update with same fingerprint but different EndsAt
		updatedAlert := model.Alert{
			Labels: model.LabelSet{"alertname": "TestAlert", "instance": "server1"},
			EndsAt: time.Now().Add(2 * time.Hour),
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			bucket.Upsert(updatedAlert.Fingerprint(), updatedAlert.EndsAt)
		}
	})

	b.Run("MixedWorkload", func(b *testing.B) {
		bucketSize := 50
		bucket := NewBucket[model.Fingerprint](bucketSize)

		// Pre-populate with mix of expired and active alerts
		for i := 0; i < bucketSize/2; i++ {
			expiredAlert := model.Alert{
				Labels: model.LabelSet{"alertname": model.LabelValue("ExpiredAlert" + string(rune(i))), "instance": "server1"},
				EndsAt: time.Now().Add(-1 * time.Hour),
			}
			bucket.Upsert(expiredAlert.Fingerprint(), expiredAlert.EndsAt)
		}
		for i := 0; i < bucketSize/2; i++ {
			activeAlert := model.Alert{
				Labels: model.LabelSet{"alertname": model.LabelValue("ActiveAlert" + string(rune(i))), "instance": "server1"},
				EndsAt: time.Now().Add(1 * time.Hour),
			}
			bucket.Upsert(activeAlert.Fingerprint(), activeAlert.EndsAt)
		}

		// Create different types of alerts for the benchmark
		alerts := []*model.Alert{
			{Labels: model.LabelSet{"alertname": "NewAlert1", "instance": "server2"}, EndsAt: time.Now().Add(1 * time.Hour)},
			{Labels: model.LabelSet{"alertname": "ExpiredAlert0", "instance": "server1"}, EndsAt: time.Now().Add(2 * time.Hour)}, // Update existing
			{Labels: model.LabelSet{"alertname": "NewAlert2", "instance": "server3"}, EndsAt: time.Now().Add(1 * time.Hour)},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			alertIndex := i % len(alerts)
			bucket.Upsert(alerts[alertIndex].Fingerprint(), alerts[alertIndex].EndsAt)
		}
	})
}

// Benchmark different bucket sizes to understand scaling behavior.
func BenchmarkBucketUpsertScaling(b *testing.B) {
	sizes := []int{10, 50, 100, 500, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("BucketSize_%d", size), func(b *testing.B) {
			bucket := NewBucket[model.Fingerprint](size)

			// Fill bucket to capacity with expired items
			for i := range size {
				alert := model.Alert{
					Labels: model.LabelSet{"alertname": model.LabelValue(fmt.Sprintf("Alert%d", i)), "instance": "server1"},
					EndsAt: time.Now().Add(-1 * time.Hour),
				}
				bucket.Upsert(alert.Fingerprint(), alert.EndsAt)
			}

			newAlert := model.Alert{
				Labels: model.LabelSet{"alertname": "NewAlert", "instance": "server2"},
				EndsAt: time.Now().Add(1 * time.Hour),
			}

			b.ResetTimer()
			for range b.N {
				bucket.Upsert(newAlert.Fingerprint(), newAlert.EndsAt)
			}
		})
	}
}

func TestBucketIsStale(t *testing.T) {
	t.Run("IsStale on empty bucket should return true", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](5)

		// Should not panic when bucket is empty and return true
		require.NotPanics(t, func() {
			stale := bucket.IsStale()
			require.True(t, stale, "IsStale on empty bucket should return true")
		}, "IsStale on empty bucket should not panic")
	})

	t.Run("IsStale returns true when latest item is expired", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](3)

		// Add three alerts that are all expired
		expiredTime := time.Now().Add(-1 * time.Hour)
		alert1 := model.Alert{Labels: model.LabelSet{"alertname": "Alert1"}, EndsAt: expiredTime}
		alert2 := model.Alert{Labels: model.LabelSet{"alertname": "Alert2"}, EndsAt: expiredTime.Add(-10 * time.Minute)}
		alert3 := model.Alert{Labels: model.LabelSet{"alertname": "Alert3"}, EndsAt: expiredTime.Add(-20 * time.Minute)}

		bucket.Upsert(alert1.Fingerprint(), alert1.EndsAt)
		bucket.Upsert(alert2.Fingerprint(), alert2.EndsAt)
		bucket.Upsert(alert3.Fingerprint(), alert3.EndsAt)

		require.Len(t, bucket.items, 3, "Bucket should have 3 items before IsStale check")
		require.Len(t, bucket.index, 3, "Bucket index should have 3 items before IsStale check")

		// IsStale should return true when all items are expired
		stale := bucket.IsStale()

		require.True(t, stale, "IsStale should return true when all items are expired")
		// IsStale doesn't remove items, so bucket should still contain them
		require.Len(t, bucket.items, 3, "Bucket should still have 3 items after IsStale check")
		require.Len(t, bucket.index, 3, "Bucket index should still have 3 items after IsStale check")
	})

	t.Run("IsStale returns false when latest item is not expired", func(t *testing.T) {
		bucket := NewBucket[model.Fingerprint](3)

		// Add mix of expired and non-expired alerts
		expiredTime := time.Now().Add(-1 * time.Hour)
		futureTime := time.Now().Add(1 * time.Hour)

		alert1 := model.Alert{Labels: model.LabelSet{"alertname": "Expired1"}, EndsAt: expiredTime}
		alert2 := model.Alert{Labels: model.LabelSet{"alertname": "Expired2"}, EndsAt: expiredTime.Add(-10 * time.Minute)}
		alert3 := model.Alert{Labels: model.LabelSet{"alertname": "Active"}, EndsAt: futureTime}

		bucket.Upsert(alert1.Fingerprint(), alert1.EndsAt)
		bucket.Upsert(alert2.Fingerprint(), alert2.EndsAt)
		bucket.Upsert(alert3.Fingerprint(), alert3.EndsAt)

		require.Len(t, bucket.items, 3, "Bucket should have 3 items before IsStale check")

		// IsStale should return false since the latest item (alert3) is not expired
		stale := bucket.IsStale()

		require.False(t, stale, "IsStale should return false when latest item is not expired")
		require.Len(t, bucket.items, 3, "Bucket should still have 3 items after IsStale check")
		require.Len(t, bucket.index, 3, "Bucket index should still have 3 items after IsStale check")
	})
}

// Benchmark concurrent access to Bucket.Upsert().
func BenchmarkBucketUpsertConcurrent(b *testing.B) {
	bucket := NewBucket[model.Fingerprint](100)

	b.RunParallel(func(pb *testing.PB) {
		alertCounter := 0
		for pb.Next() {
			alert := model.Alert{
				Labels: model.LabelSet{"alertname": model.LabelValue("Alert" + string(rune(alertCounter))), "instance": "server1"},
				EndsAt: time.Now().Add(1 * time.Hour),
			}
			bucket.Upsert(alert.Fingerprint(), alert.EndsAt)
			alertCounter++
		}
	})
}
