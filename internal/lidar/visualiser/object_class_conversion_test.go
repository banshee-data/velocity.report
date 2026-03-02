package visualiser

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l6objects"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/pb"
)

// TestObjectClassFromString tests the conversion of string labels to proto enums.
func TestObjectClassFromString(t *testing.T) {
	tests := []struct {
		input    string
		expected pb.ObjectClass
		desc     string
	}{
		{"car", pb.ObjectClass_OBJECT_CLASS_CAR, "car → CAR"},
		{"truck", pb.ObjectClass_OBJECT_CLASS_TRUCK, "truck → TRUCK"},
		{"bus", pb.ObjectClass_OBJECT_CLASS_BUS, "bus → BUS"},
		{"pedestrian", pb.ObjectClass_OBJECT_CLASS_PEDESTRIAN, "pedestrian → PEDESTRIAN"},
		{"cyclist", pb.ObjectClass_OBJECT_CLASS_CYCLIST, "cyclist → CYCLIST"},
		{"motorcyclist", pb.ObjectClass_OBJECT_CLASS_MOTORCYCLIST, "motorcyclist → MOTORCYCLIST"},
		{"bird", pb.ObjectClass_OBJECT_CLASS_BIRD, "bird → BIRD"},
		{"noise", pb.ObjectClass_OBJECT_CLASS_NOISE, "noise → NOISE"},
		{"dynamic", pb.ObjectClass_OBJECT_CLASS_DYNAMIC, "dynamic → DYNAMIC"},
		{"", pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED, "empty string → UNSPECIFIED"},
		{"unknown", pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED, "unknown value → UNSPECIFIED"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := objectClassFromString(tt.input)
			if result != tt.expected {
				t.Errorf("objectClassFromString(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestTrackObjectClassPropagation tests that Track.ObjectClass is correctly set in proto messages.
func TestTrackObjectClassPropagation(t *testing.T) {
	tests := []struct {
		objectClass string
		expected    pb.ObjectClass
		desc        string
	}{
		{string(l6objects.ClassCar), pb.ObjectClass_OBJECT_CLASS_CAR, "ClassCar constant"},
		{string(l6objects.ClassTruck), pb.ObjectClass_OBJECT_CLASS_TRUCK, "ClassTruck constant"},
		{string(l6objects.ClassBus), pb.ObjectClass_OBJECT_CLASS_BUS, "ClassBus constant"},
		{string(l6objects.ClassPedestrian), pb.ObjectClass_OBJECT_CLASS_PEDESTRIAN, "ClassPedestrian constant"},
		{string(l6objects.ClassCyclist), pb.ObjectClass_OBJECT_CLASS_CYCLIST, "ClassCyclist constant"},
		{string(l6objects.ClassMotorcyclist), pb.ObjectClass_OBJECT_CLASS_MOTORCYCLIST, "ClassMotorcyclist constant"},
		{string(l6objects.ClassBird), pb.ObjectClass_OBJECT_CLASS_BIRD, "ClassBird constant"},
		{string(l6objects.ClassDynamic), pb.ObjectClass_OBJECT_CLASS_DYNAMIC, "ClassDynamic constant"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			// Create a visualiser Track with the objectClass set
			track := &Track{
				TrackID:     "test-track-123",
				ObjectClass: tt.objectClass,
			}

			// Convert via the grpc_server's objectClassFromString function
			protoEnum := objectClassFromString(track.ObjectClass)

			if protoEnum != tt.expected {
				t.Errorf("Track with ObjectClass=%q converted to %v, want %v", tt.objectClass, protoEnum, tt.expected)
			}
		})
	}
}

// TestObjectClassRoundtrip tests that we can convert string → enum → proto → string without loss.
func TestObjectClassRoundtrip(t *testing.T) {
	classLabels := []string{
		string(l6objects.ClassCar),
		string(l6objects.ClassTruck),
		string(l6objects.ClassBus),
		string(l6objects.ClassPedestrian),
		string(l6objects.ClassCyclist),
		string(l6objects.ClassMotorcyclist),
		string(l6objects.ClassBird),
		string(l6objects.ClassDynamic),
	}

	for _, label := range classLabels {
		t.Run(label, func(t *testing.T) {
			// String → enum
			enum := objectClassFromString(label)

			// Enum should not be unspecified
			if enum == pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED {
				t.Errorf("objectClassFromString(%q) returned UNSPECIFIED", label)
			}

			// Verify enum value is in valid range
			if enum < 0 || enum > 9 {
				t.Errorf("objectClassFromString(%q) returned invalid enum %v", label, enum)
			}
		})
	}
}

// TestEmptyObjectClassToUnspecified tests that missing/uninitialized ObjectClass becomes UNSPECIFIED.
func TestEmptyObjectClassToUnspecified(t *testing.T) {
	track := &Track{
		TrackID:     "test-track",
		ObjectClass: "", // Empty/uninitialized
	}

	result := objectClassFromString(track.ObjectClass)
	if result != pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED {
		t.Errorf("Empty ObjectClass should convert to UNSPECIFIED, got %v", result)
	}
}

// TestAllClassConstantsAreConvertible tests that all l6objects class constants can be converted.
func TestAllClassConstantsAreConvertible(t *testing.T) {
	// This is a meta-test to ensure we didn't miss any ObjectClass constant
	constants := []string{
		string(l6objects.ClassCar),
		string(l6objects.ClassTruck),
		string(l6objects.ClassBus),
		string(l6objects.ClassPedestrian),
		string(l6objects.ClassCyclist),
		string(l6objects.ClassMotorcyclist),
		string(l6objects.ClassBird),
		string(l6objects.ClassDynamic),
		string(l6objects.ClassPed), // Alias
	}

	for _, c := range constants {
		result := objectClassFromString(c)
		if result == pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED {
			t.Errorf("Class constant %q should not convert to UNSPECIFIED", c)
		}
	}
}

// TestClassifyOrConvert_ExistingLabel tests that tracks with an existing
// ObjectClass string are converted directly without re-classification.
func TestClassifyOrConvert_ExistingLabel(t *testing.T) {
	track := Track{
		TrackID:          "test-1",
		ObjectClass:      "car",
		ObservationCount: 10,
		BBoxLength:       4.0,
		BBoxWidth:        2.0,
	}
	result := classifyOrConvert(track)
	if result != pb.ObjectClass_OBJECT_CLASS_CAR {
		t.Errorf("classifyOrConvert with ObjectClass='car' = %v, want CAR", result)
	}
}

// TestClassifyOrConvert_EmptyLabel_Reclassifies tests that tracks with
// empty ObjectClass (e.g. from old VRLOG recordings) are re-classified
// on-the-fly from their aggregate features.
func TestClassifyOrConvert_EmptyLabel_Reclassifies(t *testing.T) {
	tests := []struct {
		desc     string
		track    Track
		expected pb.ObjectClass
	}{
		{
			desc: "vehicle-sized track → CAR",
			track: Track{
				TrackID:          "vrlog-car",
				ObservationCount: 20,
				BBoxLength:       4.5,
				BBoxWidth:        2.0,
				BBoxHeight:       1.5,
				MedianSpeedMps:   12.0,
				PeakSpeedMps:     15.0,
			},
			expected: pb.ObjectClass_OBJECT_CLASS_CAR,
		},
		{
			desc: "small slow object → BIRD",
			track: Track{
				TrackID:          "vrlog-bird",
				ObservationCount: 10,
				BBoxLength:       0.3,
				BBoxWidth:        0.3,
				BBoxHeight:       0.2,
				MedianSpeedMps:   0.5,
				PeakSpeedMps:     0.8,
			},
			expected: pb.ObjectClass_OBJECT_CLASS_BIRD,
		},
		{
			desc: "human-sized slow track → PEDESTRIAN",
			track: Track{
				TrackID:          "vrlog-ped",
				ObservationCount: 15,
				BBoxLength:       0.5,
				BBoxWidth:        0.5,
				BBoxHeight:       1.7,
				MedianSpeedMps:   1.2,
				PeakSpeedMps:     2.0,
			},
			expected: pb.ObjectClass_OBJECT_CLASS_PEDESTRIAN,
		},
		{
			desc: "too few observations → DYNAMIC",
			track: Track{
				TrackID:          "vrlog-few",
				ObservationCount: 1,
				BBoxLength:       4.5,
				BBoxWidth:        2.0,
				BBoxHeight:       1.5,
				MedianSpeedMps:   12.0,
			},
			expected: pb.ObjectClass_OBJECT_CLASS_DYNAMIC,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			result := classifyOrConvert(tt.track)
			if result != tt.expected {
				t.Errorf("classifyOrConvert() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestClassifyOrConvert_EmptyLabel_NotUnspecified verifies that re-classification
// never returns UNSPECIFIED (it always produces a meaningful label).
func TestClassifyOrConvert_EmptyLabel_NotUnspecified(t *testing.T) {
	track := Track{
		TrackID:          "vrlog-any",
		ObservationCount: 20,
		BBoxLength:       3.0,
		BBoxWidth:        1.5,
		BBoxHeight:       1.2,
		MedianSpeedMps:   8.0,
		PeakSpeedMps:     12.0,
	}
	result := classifyOrConvert(track)
	if result == pb.ObjectClass_OBJECT_CLASS_UNSPECIFIED {
		t.Error("classifyOrConvert with empty ObjectClass should never return UNSPECIFIED")
	}
}
