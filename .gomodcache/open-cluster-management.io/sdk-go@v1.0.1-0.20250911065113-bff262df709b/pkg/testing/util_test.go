package testing

import (
	"os"
	"testing"
)

// TestWriteToTempFile tests the WriteToTempFile function
func TestWriteToTempFile(t *testing.T) {
	testCases := []struct {
		name     string
		data     []byte
		pattern  string
		expected string
	}{
		{
			name:     "Empty data",
			data:     []byte(""),
			pattern:  "test*.txt",
			expected: "",
		},
		{
			name:     "Empty pattern",
			data:     []byte("Hello, World!"),
			pattern:  "",
			expected: "Hello, World!",
		},
		{
			name:     "Specific pattern",
			data:     []byte("Hello, World!"),
			pattern:  "testfile*.txt",
			expected: "Hello, World!",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := WriteToTempFile(tc.pattern, tc.data)
			if err != nil {
				t.Errorf("Error writing to temporary file: %v", err)
			}
			defer os.Remove(file.Name())

			// Verify that the file exists
			if _, err := os.Stat(file.Name()); os.IsNotExist(err) {
				t.Errorf("Temporary file does not exist: %s", file.Name())
			}

			// Verify the content of the file
			content, err := os.ReadFile(file.Name())
			if err != nil {
				t.Errorf("Error reading temporary file: %v", err)
			}

			if string(content) != tc.expected {
				t.Errorf("Unexpected content in temporary file. Expected: %s, Got: %s", tc.expected, string(content))
			}
		})
	}
}

// TestRemoveTempFile tests the RemoveTempFile function
func TestRemoveTempFile(t *testing.T) {
	testCases := []struct {
		name     string
		file     string
		existing bool
		expected bool
	}{
		{
			name:     "File exists",
			file:     "/tmp/testfile.txt",
			existing: true,
		},
		{
			name:     "File does not exist",
			file:     "/tmp/nonexistentfile.txt",
			existing: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create the file if it should exist
			if tc.existing {
				f, err := os.Create(tc.file)
				if err != nil {
					t.Errorf("Error creating file: %v", err)
				}
				defer f.Close()
			}

			err := RemoveTempFile(tc.file)
			if err != nil {
				t.Errorf("Error removing temporary file: %v", err)
			}

			// Verify that the file does not exist
			_, err = os.Stat(tc.file)
			if !os.IsNotExist(err) {
				t.Errorf("Temporary file still exists: %s", tc.file)
			}
		})
	}
}
