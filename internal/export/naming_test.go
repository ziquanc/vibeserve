package export

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Car Rental API", "car-rental-api"},
		{"malaysia-car-rental", "malaysia-car-rental"},
		{"My Cool App!", "my-cool-app"},
		{"  spaces  everywhere  ", "spaces-everywhere"},
		{"UPPER_CASE", "upper-case"},
	}
	for _, tt := range tests {
		got := Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"vehicles", "vehicle"},
		{"bookings", "booking"},
		{"categories", "category"},
		{"statuses", "status"},
		{"users", "user"},
		{"person", "person"},
	}
	for _, tt := range tests {
		got := Singularize(tt.input)
		if got != tt.want {
			t.Errorf("Singularize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPascalCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"vehicle", "Vehicle"},
		{"daily_rate", "DailyRate"},
		{"total_price", "TotalPrice"},
		{"id", "ID"},
		{"vehicle_id", "VehicleID"},
		{"api_url", "APIURL"},
	}
	for _, tt := range tests {
		got := PascalCase(tt.input)
		if got != tt.want {
			t.Errorf("PascalCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCamelCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"vehicle", "vehicle"},
		{"daily_rate", "dailyRate"},
		{"total_price", "totalPrice"},
		{"id", "id"},
	}
	for _, tt := range tests {
		got := CamelCase(tt.input)
		if got != tt.want {
			t.Errorf("CamelCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTableToStructName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"vehicles", "Vehicle"},
		{"bookings", "Booking"},
		{"categories", "Category"},
		{"order_items", "OrderItem"},
	}
	for _, tt := range tests {
		got := TableToStructName(tt.input)
		if got != tt.want {
			t.Errorf("TableToStructName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
