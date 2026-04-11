package export

import (
	"strings"
	"testing"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

func TestGenerateTypeScript(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "users",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
			{Name: "email", Type: "TEXT", Required: true, Unique: true},
			{Name: "age", Type: "INTEGER"},
			{Name: "active", Type: "BOOLEAN"},
			{Name: "score", Type: "REAL"},
		},
	}}

	result := GenerateTypeScript(schemas)

	// Main interface
	if !strings.Contains(result, "export interface User {") {
		t.Error("should generate User interface")
	}
	if !strings.Contains(result, "id: number;") {
		t.Error("INTEGER should map to number")
	}
	if !strings.Contains(result, "name: string;") {
		t.Error("TEXT should map to string")
	}
	if !strings.Contains(result, "active: boolean;") {
		t.Error("BOOLEAN should map to boolean")
	}
	if !strings.Contains(result, "score: number;") {
		t.Error("REAL should map to number")
	}
	if !strings.Contains(result, "deleted_at: string | null;") {
		t.Error("deleted_at should be string | null")
	}

	// Create input
	if !strings.Contains(result, "export interface CreateUserInput {") {
		t.Error("should generate CreateUserInput")
	}

	// Update input
	if !strings.Contains(result, "export interface UpdateUserInput {") {
		t.Error("should generate UpdateUserInput")
	}

	// ListOptions
	if !strings.Contains(result, "export interface ListOptions {") {
		t.Error("should generate ListOptions")
	}

	// Timestamp columns should be in main interface but not in create/update
	if !strings.Contains(result, "created_at: string;") {
		t.Error("main interface should have created_at")
	}
}

func TestGenerateTypeScript_RequiredFields(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "posts",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "title", Type: "TEXT", Required: true},
			{Name: "body", Type: "TEXT"},
		},
	}}

	result := GenerateTypeScript(schemas)

	// In CreatePostInput, title should be required (no ?), body should be optional
	if !strings.Contains(result, "title: string;") {
		t.Error("required field should not have ? in CreateInput")
	}
	if !strings.Contains(result, "body?: string;") {
		t.Error("optional field should have ? in CreateInput")
	}
}

func TestGenerateTypeScript_CreateInputExcludesPKAndTimestamps(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "items",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "label", Type: "TEXT", Required: true},
		},
	}}

	result := GenerateTypeScript(schemas)

	// Extract the CreateItemInput block
	createStart := strings.Index(result, "export interface CreateItemInput {")
	if createStart == -1 {
		t.Fatal("CreateItemInput not found")
	}
	createEnd := strings.Index(result[createStart:], "}\n")
	createBlock := result[createStart : createStart+createEnd]

	if strings.Contains(createBlock, "id") {
		t.Error("CreateInput should not contain auto PK column 'id'")
	}
	if strings.Contains(createBlock, "created_at") {
		t.Error("CreateInput should not contain timestamp 'created_at'")
	}
	if strings.Contains(createBlock, "updated_at") {
		t.Error("CreateInput should not contain timestamp 'updated_at'")
	}
	if strings.Contains(createBlock, "deleted_at") {
		t.Error("CreateInput should not contain timestamp 'deleted_at'")
	}
}

func TestGenerateTypeScript_UpdateInputAllOptional(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "products",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "name", Type: "TEXT", Required: true},
			{Name: "price", Type: "REAL", Required: true},
		},
	}}

	result := GenerateTypeScript(schemas)

	// Extract the UpdateProductInput block
	updateStart := strings.Index(result, "export interface UpdateProductInput {")
	if updateStart == -1 {
		t.Fatal("UpdateProductInput not found")
	}
	updateEnd := strings.Index(result[updateStart:], "}\n")
	updateBlock := result[updateStart : updateStart+updateEnd]

	if !strings.Contains(updateBlock, "name?: string;") {
		t.Error("UpdateInput fields should all be optional")
	}
	if !strings.Contains(updateBlock, "price?: number;") {
		t.Error("UpdateInput fields should all be optional")
	}
	if strings.Contains(updateBlock, "id") {
		t.Error("UpdateInput should not contain PK column")
	}
}

func TestGenerateTypeScript_MultipleSchemas(t *testing.T) {
	schemas := []manifest.Schema{
		{
			Table: "users",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "name", Type: "TEXT", Required: true},
			},
		},
		{
			Table: "posts",
			Columns: []manifest.Column{
				{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
				{Name: "title", Type: "TEXT", Required: true},
			},
		},
	}

	result := GenerateTypeScript(schemas)

	if !strings.Contains(result, "export interface User {") {
		t.Error("should generate User interface")
	}
	if !strings.Contains(result, "export interface Post {") {
		t.Error("should generate Post interface")
	}
	if !strings.Contains(result, "export interface CreateUserInput {") {
		t.Error("should generate CreateUserInput")
	}
	if !strings.Contains(result, "export interface CreatePostInput {") {
		t.Error("should generate CreatePostInput")
	}

	// ListOptions should appear only once
	count := strings.Count(result, "export interface ListOptions {")
	if count != 1 {
		t.Errorf("ListOptions should appear exactly once, got %d", count)
	}
}

func TestGenerateTypeScript_TypeMapping(t *testing.T) {
	schemas := []manifest.Schema{{
		Table: "events",
		Columns: []manifest.Column{
			{Name: "id", Type: "INTEGER", Primary: true, Auto: true},
			{Name: "date_field", Type: "DATE"},
			{Name: "datetime_field", Type: "DATETIME"},
			{Name: "unknown_field", Type: "BLOB"},
		},
	}}

	result := GenerateTypeScript(schemas)

	if !strings.Contains(result, "date_field: string;") {
		t.Error("DATE should map to string")
	}
	if !strings.Contains(result, "datetime_field: string;") {
		t.Error("DATETIME should map to string")
	}
	if !strings.Contains(result, "unknown_field: any;") {
		t.Error("unknown types should map to any")
	}
}

func TestGenerateTSConfig(t *testing.T) {
	result := GenerateTSConfig()
	if !strings.Contains(result, "compilerOptions") {
		t.Error("should contain compilerOptions")
	}
	if !strings.Contains(result, "ES2020") {
		t.Error("should target ES2020")
	}
	if !strings.Contains(result, `"declaration": true`) {
		t.Error("should enable declaration generation")
	}
	if !strings.Contains(result, `"esModuleInterop": true`) {
		t.Error("should enable esModuleInterop")
	}
}
