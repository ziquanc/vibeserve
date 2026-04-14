// Package templates provides pre-built API manifests for common use cases.
package templates

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/vibeserve/vibeserve/internal/manifest"
)

// Template holds a pre-built manifest with metadata.
type Template struct {
	Name        string
	Description string
	Manifest    *manifest.Manifest
}

// List returns all available templates.
func List() []Template {
	return []Template{
		{Name: "blog", Description: "Blog with users, posts, comments, categories, and tags"},
		{Name: "ecommerce", Description: "E-commerce store with products, orders, cart, reviews, and coupons"},
		{Name: "saas", Description: "SaaS starter with teams, members, plans, subscriptions, and billing"},
	}
}

// Get returns a template by name, or error if not found.
func Get(name string) (*Template, error) {
	lower := strings.ToLower(name)
	for _, t := range List() {
		if t.Name == lower {
			m := buildManifest(lower)
			t.Manifest = m
			return &t, nil
		}
	}
	available := make([]string, 0)
	for _, t := range List() {
		available = append(available, t.Name)
	}
	return nil, fmt.Errorf("unknown template %q (available: %s)", name, strings.Join(available, ", "))
}

func buildManifest(name string) *manifest.Manifest {
	var raw string
	switch name {
	case "blog":
		raw = blogManifest
	case "ecommerce":
		raw = ecommerceManifest
	case "saas":
		raw = saasManifest
	default:
		return nil
	}
	var m manifest.Manifest
	json.Unmarshal([]byte(raw), &m)
	return &m
}

var blogManifest = `{
  "version": "1.0",
  "name": "blog-api",
  "description": "Blog API with posts, comments, categories, and tags",
  "schemas": [
    {
      "table": "users",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "email", "type": "TEXT", "required": true, "unique": true},
        {"name": "password_hash", "type": "TEXT", "required": true},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "role", "type": "TEXT", "required": true, "default": "user"},
        {"name": "bio", "type": "TEXT"},
        {"name": "avatar_url", "type": "TEXT"}
      ]
    },
    {
      "table": "categories",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "name", "type": "TEXT", "required": true, "unique": true},
        {"name": "slug", "type": "TEXT", "required": true, "unique": true},
        {"name": "description", "type": "TEXT"}
      ]
    },
    {
      "table": "posts",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "author_id", "type": "INTEGER", "required": true, "references": "users.id"},
        {"name": "category_id", "type": "INTEGER", "references": "categories.id"},
        {"name": "title", "type": "TEXT", "required": true},
        {"name": "slug", "type": "TEXT", "required": true, "unique": true},
        {"name": "content", "type": "TEXT", "required": true},
        {"name": "excerpt", "type": "TEXT"},
        {"name": "status", "type": "TEXT", "required": true, "default": "draft"},
        {"name": "published_at", "type": "DATETIME"}
      ],
      "state_machine": {
        "field": "status",
        "initial": "draft",
        "transitions": [
          {"from": "draft", "to": "published", "action": "publish", "guard": {"role": "admin"}},
          {"from": "published", "to": "draft", "action": "unpublish", "guard": {"role": "admin"}},
          {"from": "draft", "to": "archived", "action": "archive"},
          {"from": "published", "to": "archived", "action": "archive"}
        ]
      }
    },
    {
      "table": "tags",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "name", "type": "TEXT", "required": true, "unique": true},
        {"name": "slug", "type": "TEXT", "required": true, "unique": true}
      ]
    },
    {
      "table": "post_tags",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "post_id", "type": "INTEGER", "required": true, "references": "posts.id"},
        {"name": "tag_id", "type": "INTEGER", "required": true, "references": "tags.id"}
      ]
    },
    {
      "table": "comments",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "post_id", "type": "INTEGER", "required": true, "references": "posts.id"},
        {"name": "user_id", "type": "INTEGER", "required": true, "references": "users.id"},
        {"name": "parent_id", "type": "INTEGER", "references": "comments.id"},
        {"name": "content", "type": "TEXT", "required": true},
        {"name": "status", "type": "TEXT", "required": true, "default": "pending"}
      ],
      "state_machine": {
        "field": "status",
        "initial": "pending",
        "transitions": [
          {"from": "pending", "to": "approved", "action": "approve", "guard": {"role": "admin"}},
          {"from": "pending", "to": "spam", "action": "mark_spam", "guard": {"role": "admin"}}
        ]
      }
    }
  ],
  "routes": [
    {"path": "/posts", "method": "GET", "description": "List published posts", "script": "list_posts", "response_type": "array"},
    {"path": "/posts/:id", "method": "GET", "description": "Get a post by ID", "script": "get_post", "response_type": "object"},
    {"path": "/posts", "method": "POST", "description": "Create a post", "script": "create_post", "response_type": "object"},
    {"path": "/posts/:id", "method": "PUT", "description": "Update a post", "script": "update_post", "response_type": "object"},
    {"path": "/posts/:id", "method": "DELETE", "description": "Delete a post", "script": "delete_post", "response_type": "object"},
    {"path": "/posts/:id/comments", "method": "GET", "description": "List comments on a post", "script": "list_post_comments", "response_type": "array"},
    {"path": "/posts/:id/comments", "method": "POST", "description": "Add a comment to a post", "script": "create_comment", "response_type": "object"},
    {"path": "/posts/:id/tags", "method": "POST", "description": "Add a tag to a post", "script": "add_post_tag", "response_type": "object"},
    {"path": "/posts/:id/tags/:tagId", "method": "DELETE", "description": "Remove a tag from a post", "script": "remove_post_tag", "response_type": "object"},
    {"path": "/categories", "method": "GET", "description": "List categories", "script": "list_categories", "response_type": "array"},
    {"path": "/categories", "method": "POST", "description": "Create a category", "script": "create_category", "response_type": "object"},
    {"path": "/tags", "method": "GET", "description": "List tags", "script": "list_tags", "response_type": "array"},
    {"path": "/tags", "method": "POST", "description": "Create a tag", "script": "create_tag", "response_type": "object"},
    {"path": "/admin/posts", "method": "GET", "description": "Admin: list all posts", "script": "admin_list_posts", "response_type": "array"},
    {"path": "/admin/comments", "method": "GET", "description": "Admin: list pending comments", "script": "admin_list_comments", "response_type": "array"},
    {"path": "/admin/dashboard", "method": "GET", "description": "Admin dashboard stats", "script": "admin_dashboard", "response_type": "object"}
  ],
  "scripts": [
    {"name": "list_posts", "code": "result := db.query(\"SELECT * FROM posts WHERE status = 'published' AND deleted_at IS NULL ORDER BY published_at DESC\", [])\nresponse.json(result)"},
    {"name": "get_post", "code": "id := request.param(\"id\")\npost := db.query_one(\"SELECT * FROM posts WHERE id = ? AND deleted_at IS NULL\", [id])\nif post == undefined {\n  response.fail(404, \"post not found\")\n}\nresponse.json(post)"},
    {"name": "create_post", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nbody := request.body()\nbody.author_id = me.user_id\nresult := db.insert(\"posts\", body)\nresponse.json(result, 201)"},
    {"name": "update_post", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nid := request.param(\"id\")\nbody := request.body()\nresult := db.update(\"posts\", id, body)\nresponse.json(result)"},
    {"name": "delete_post", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nid := request.param(\"id\")\ndb.delete(\"posts\", id)\nresponse.json({\"deleted\": true})"},
    {"name": "list_post_comments", "code": "id := request.param(\"id\")\nresult := db.query(\"SELECT * FROM comments WHERE post_id = ? AND status = 'approved' AND deleted_at IS NULL\", [id])\nresponse.json(result)"},
    {"name": "create_comment", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nid := request.param(\"id\")\nbody := request.body()\nbody.post_id = id\nbody.user_id = me.user_id\nresult := db.insert(\"comments\", body)\nresponse.json(result, 201)"},
    {"name": "add_post_tag", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nbody := request.body()\nbody.post_id = request.param(\"id\")\nresult := db.insert(\"post_tags\", body)\nresponse.json(result, 201)"},
    {"name": "remove_post_tag", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\ndb.delete(\"post_tags\", request.param(\"tagId\"))\nresponse.json({\"removed\": true})"},
    {"name": "list_categories", "code": "result := db.query(\"SELECT * FROM categories WHERE deleted_at IS NULL\", [])\nresponse.json(result)"},
    {"name": "create_category", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nresult := db.insert(\"categories\", request.body())\nresponse.json(result, 201)"},
    {"name": "list_tags", "code": "result := db.query(\"SELECT * FROM tags WHERE deleted_at IS NULL\", [])\nresponse.json(result)"},
    {"name": "create_tag", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nresult := db.insert(\"tags\", request.body())\nresponse.json(result, 201)"},
    {"name": "admin_list_posts", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nresult := db.query(\"SELECT * FROM posts WHERE deleted_at IS NULL\", [])\nresponse.json(result)"},
    {"name": "admin_list_comments", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nresult := db.query(\"SELECT * FROM comments WHERE status = 'pending' AND deleted_at IS NULL\", [])\nresponse.json(result)"},
    {"name": "admin_dashboard", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nposts := db.count(\"posts\")\ncomments := db.count(\"comments\")\nusers := db.count(\"users\")\nresponse.json({\"posts\": posts, \"comments\": comments, \"users\": users})"}
  ],
  "seeds": [
    {"table": "users", "rows": [
      {"email": "admin@example.com", "password_hash": "$2a$10$placeholder", "name": "Admin", "role": "admin"},
      {"email": "writer@example.com", "password_hash": "$2a$10$placeholder", "name": "Writer", "role": "user"}
    ]},
    {"table": "categories", "rows": [
      {"name": "Technology", "slug": "technology", "description": "Tech news and tutorials"},
      {"name": "Lifestyle", "slug": "lifestyle", "description": "Life tips and stories"}
    ]},
    {"table": "tags", "rows": [
      {"name": "JavaScript", "slug": "javascript"},
      {"name": "Go", "slug": "go"},
      {"name": "Tutorial", "slug": "tutorial"}
    ]}
  ]
}`

var ecommerceManifest = `{
  "version": "1.0",
  "name": "ecommerce-api",
  "description": "E-commerce store with products, orders, cart, reviews, and coupons",
  "schemas": [
    {
      "table": "users",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "email", "type": "TEXT", "required": true, "unique": true},
        {"name": "password_hash", "type": "TEXT", "required": true},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "role", "type": "TEXT", "required": true, "default": "customer"},
        {"name": "phone", "type": "TEXT"},
        {"name": "address", "type": "TEXT"}
      ]
    },
    {
      "table": "categories",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "slug", "type": "TEXT", "required": true, "unique": true},
        {"name": "parent_id", "type": "INTEGER", "references": "categories.id"}
      ]
    },
    {
      "table": "products",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "category_id", "type": "INTEGER", "references": "categories.id"},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "slug", "type": "TEXT", "required": true, "unique": true},
        {"name": "description", "type": "TEXT"},
        {"name": "price", "type": "REAL", "required": true},
        {"name": "stock", "type": "INTEGER", "required": true, "default": 0},
        {"name": "image_url", "type": "TEXT"},
        {"name": "active", "type": "BOOLEAN", "default": true}
      ]
    },
    {
      "table": "cart_items",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "user_id", "type": "INTEGER", "required": true, "references": "users.id"},
        {"name": "product_id", "type": "INTEGER", "required": true, "references": "products.id"},
        {"name": "quantity", "type": "INTEGER", "required": true, "default": 1}
      ]
    },
    {
      "table": "orders",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "user_id", "type": "INTEGER", "required": true, "references": "users.id"},
        {"name": "status", "type": "TEXT", "required": true, "default": "pending"},
        {"name": "total", "type": "REAL", "required": true},
        {"name": "shipping_address", "type": "TEXT"},
        {"name": "coupon_code", "type": "TEXT"}
      ],
      "state_machine": {
        "field": "status",
        "initial": "pending",
        "transitions": [
          {"from": "pending", "to": "confirmed", "action": "confirm", "guard": {"role": "admin"}},
          {"from": "confirmed", "to": "shipped", "action": "ship", "guard": {"role": "admin"}},
          {"from": "shipped", "to": "delivered", "action": "deliver"},
          {"from": "pending", "to": "cancelled", "action": "cancel"},
          {"from": "confirmed", "to": "cancelled", "action": "cancel"}
        ]
      }
    },
    {
      "table": "order_items",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "order_id", "type": "INTEGER", "required": true, "references": "orders.id"},
        {"name": "product_id", "type": "INTEGER", "required": true, "references": "products.id"},
        {"name": "quantity", "type": "INTEGER", "required": true},
        {"name": "price", "type": "REAL", "required": true}
      ]
    },
    {
      "table": "reviews",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "product_id", "type": "INTEGER", "required": true, "references": "products.id"},
        {"name": "user_id", "type": "INTEGER", "required": true, "references": "users.id"},
        {"name": "rating", "type": "INTEGER", "required": true},
        {"name": "title", "type": "TEXT"},
        {"name": "body", "type": "TEXT"}
      ]
    },
    {
      "table": "coupons",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "code", "type": "TEXT", "required": true, "unique": true},
        {"name": "discount_percent", "type": "REAL"},
        {"name": "discount_amount", "type": "REAL"},
        {"name": "min_order", "type": "REAL", "default": 0},
        {"name": "max_uses", "type": "INTEGER"},
        {"name": "used_count", "type": "INTEGER", "default": 0},
        {"name": "expires_at", "type": "DATETIME"},
        {"name": "active", "type": "BOOLEAN", "default": true}
      ]
    }
  ],
  "routes": [
    {"path": "/products", "method": "GET", "description": "List products", "script": "list_products", "response_type": "array"},
    {"path": "/products/:id", "method": "GET", "description": "Get product", "script": "get_product", "response_type": "object"},
    {"path": "/products/:id/reviews", "method": "GET", "description": "List product reviews", "script": "list_reviews", "response_type": "array"},
    {"path": "/products/:id/reviews", "method": "POST", "description": "Add review", "script": "create_review", "response_type": "object"},
    {"path": "/categories", "method": "GET", "description": "List categories", "script": "list_categories", "response_type": "array"},
    {"path": "/cart", "method": "GET", "description": "Get my cart", "script": "get_cart", "response_type": "array"},
    {"path": "/cart", "method": "POST", "description": "Add to cart", "script": "add_to_cart", "response_type": "object"},
    {"path": "/cart/:id", "method": "DELETE", "description": "Remove from cart", "script": "remove_from_cart", "response_type": "object"},
    {"path": "/orders", "method": "POST", "description": "Place order from cart", "script": "place_order", "response_type": "object"},
    {"path": "/orders", "method": "GET", "description": "My orders", "script": "my_orders", "response_type": "array"},
    {"path": "/orders/:id", "method": "GET", "description": "Get order details", "script": "get_order", "response_type": "object"},
    {"path": "/coupons/validate/:code", "method": "GET", "description": "Validate coupon", "script": "validate_coupon", "response_type": "object"},
    {"path": "/admin/products", "method": "POST", "description": "Create product", "script": "admin_create_product", "response_type": "object"},
    {"path": "/admin/products/:id", "method": "PUT", "description": "Update product", "script": "admin_update_product", "response_type": "object"},
    {"path": "/admin/orders", "method": "GET", "description": "All orders", "script": "admin_list_orders", "response_type": "array"},
    {"path": "/admin/dashboard", "method": "GET", "description": "Admin dashboard", "script": "admin_dashboard", "response_type": "object"}
  ],
  "scripts": [
    {"name": "list_products", "code": "result := db.query(\"SELECT * FROM products WHERE active = 1 AND deleted_at IS NULL\", [])\nresponse.json(result)"},
    {"name": "get_product", "code": "id := request.param(\"id\")\np := db.query_one(\"SELECT * FROM products WHERE id = ? AND deleted_at IS NULL\", [id])\nif p == undefined {\n  response.fail(404, \"product not found\")\n}\nresponse.json(p)"},
    {"name": "list_reviews", "code": "id := request.param(\"id\")\nresult := db.query(\"SELECT * FROM reviews WHERE product_id = ? AND deleted_at IS NULL\", [id])\nresponse.json(result)"},
    {"name": "create_review", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nbody := request.body()\nbody.product_id = request.param(\"id\")\nbody.user_id = me.user_id\nresult := db.insert(\"reviews\", body)\nresponse.json(result, 201)"},
    {"name": "list_categories", "code": "result := db.query(\"SELECT * FROM categories WHERE deleted_at IS NULL\", [])\nresponse.json(result)"},
    {"name": "get_cart", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nresult := db.query(\"SELECT c.*, p.name, p.price, p.image_url FROM cart_items c JOIN products p ON c.product_id = p.id WHERE c.user_id = ? AND c.deleted_at IS NULL\", [me.user_id])\nresponse.json(result)"},
    {"name": "add_to_cart", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nbody := request.body()\nbody.user_id = me.user_id\nresult := db.insert(\"cart_items\", body)\nresponse.json(result, 201)"},
    {"name": "remove_from_cart", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\ndb.delete(\"cart_items\", request.param(\"id\"))\nresponse.json({\"removed\": true})"},
    {"name": "place_order", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nbody := request.body()\nbody.user_id = me.user_id\nresult := db.insert(\"orders\", body)\nresponse.json(result, 201)"},
    {"name": "my_orders", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nresult := db.query(\"SELECT * FROM orders WHERE user_id = ? AND deleted_at IS NULL ORDER BY created_at DESC\", [me.user_id])\nresponse.json(result)"},
    {"name": "get_order", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nid := request.param(\"id\")\norder := db.query_one(\"SELECT * FROM orders WHERE id = ? AND user_id = ? AND deleted_at IS NULL\", [id, me.user_id])\nif order == undefined {\n  response.fail(404, \"order not found\")\n}\nresponse.json(order)"},
    {"name": "validate_coupon", "code": "code := request.param(\"code\")\ncoupon := db.query_one(\"SELECT * FROM coupons WHERE code = ? AND active = 1 AND deleted_at IS NULL\", [code])\nif coupon == undefined {\n  response.fail(404, \"invalid coupon\")\n}\nresponse.json(coupon)"},
    {"name": "admin_create_product", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nresult := db.insert(\"products\", request.body())\nresponse.json(result, 201)"},
    {"name": "admin_update_product", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nresult := db.update(\"products\", request.param(\"id\"), request.body())\nresponse.json(result)"},
    {"name": "admin_list_orders", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nresult := db.query(\"SELECT * FROM orders WHERE deleted_at IS NULL ORDER BY created_at DESC\", [])\nresponse.json(result)"},
    {"name": "admin_dashboard", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nproducts := db.count(\"products\")\norders := db.count(\"orders\")\nusers := db.count(\"users\")\nresponse.json({\"products\": products, \"orders\": orders, \"users\": users})"}
  ],
  "seeds": [
    {"table": "users", "rows": [
      {"email": "admin@shop.com", "password_hash": "$2a$10$placeholder", "name": "Shop Admin", "role": "admin"},
      {"email": "customer@example.com", "password_hash": "$2a$10$placeholder", "name": "John Customer", "role": "customer"}
    ]},
    {"table": "categories", "rows": [
      {"name": "Electronics", "slug": "electronics"},
      {"name": "Clothing", "slug": "clothing"},
      {"name": "Books", "slug": "books"}
    ]},
    {"table": "products", "rows": [
      {"category_id": 1, "name": "Wireless Headphones", "slug": "wireless-headphones", "price": 49.99, "stock": 100, "active": true},
      {"category_id": 1, "name": "USB-C Cable", "slug": "usb-c-cable", "price": 9.99, "stock": 500, "active": true},
      {"category_id": 2, "name": "Cotton T-Shirt", "slug": "cotton-tshirt", "price": 19.99, "stock": 200, "active": true}
    ]},
    {"table": "coupons", "rows": [
      {"code": "WELCOME10", "discount_percent": 10, "min_order": 20, "active": true},
      {"code": "SAVE5", "discount_amount": 5, "min_order": 30, "active": true}
    ]}
  ]
}`

var saasManifest = `{
  "version": "1.0",
  "name": "saas-api",
  "description": "SaaS starter with teams, members, plans, subscriptions, and billing",
  "schemas": [
    {
      "table": "users",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "email", "type": "TEXT", "required": true, "unique": true},
        {"name": "password_hash", "type": "TEXT", "required": true},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "role", "type": "TEXT", "required": true, "default": "user"},
        {"name": "email_verified", "type": "BOOLEAN", "default": false}
      ]
    },
    {
      "table": "teams",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "slug", "type": "TEXT", "required": true, "unique": true},
        {"name": "owner_id", "type": "INTEGER", "required": true, "references": "users.id"},
        {"name": "plan_id", "type": "INTEGER", "references": "plans.id"}
      ]
    },
    {
      "table": "team_members",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "team_id", "type": "INTEGER", "required": true, "references": "teams.id"},
        {"name": "user_id", "type": "INTEGER", "required": true, "references": "users.id"},
        {"name": "role", "type": "TEXT", "required": true, "default": "member"}
      ]
    },
    {
      "table": "plans",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "slug", "type": "TEXT", "required": true, "unique": true},
        {"name": "price_monthly", "type": "REAL", "required": true},
        {"name": "price_yearly", "type": "REAL"},
        {"name": "max_members", "type": "INTEGER", "required": true},
        {"name": "features", "type": "TEXT"},
        {"name": "active", "type": "BOOLEAN", "default": true}
      ]
    },
    {
      "table": "subscriptions",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "team_id", "type": "INTEGER", "required": true, "references": "teams.id"},
        {"name": "plan_id", "type": "INTEGER", "required": true, "references": "plans.id"},
        {"name": "status", "type": "TEXT", "required": true, "default": "active"},
        {"name": "billing_cycle", "type": "TEXT", "required": true, "default": "monthly"},
        {"name": "current_period_start", "type": "DATETIME"},
        {"name": "current_period_end", "type": "DATETIME"}
      ],
      "state_machine": {
        "field": "status",
        "initial": "active",
        "transitions": [
          {"from": "active", "to": "past_due", "action": "mark_past_due"},
          {"from": "past_due", "to": "active", "action": "reactivate"},
          {"from": "active", "to": "cancelled", "action": "cancel"},
          {"from": "past_due", "to": "cancelled", "action": "cancel"}
        ]
      }
    },
    {
      "table": "invoices",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "subscription_id", "type": "INTEGER", "required": true, "references": "subscriptions.id"},
        {"name": "amount", "type": "REAL", "required": true},
        {"name": "status", "type": "TEXT", "required": true, "default": "pending"},
        {"name": "due_date", "type": "DATETIME"},
        {"name": "paid_at", "type": "DATETIME"}
      ],
      "state_machine": {
        "field": "status",
        "initial": "pending",
        "transitions": [
          {"from": "pending", "to": "paid", "action": "pay"},
          {"from": "pending", "to": "overdue", "action": "mark_overdue"},
          {"from": "overdue", "to": "paid", "action": "pay"}
        ]
      }
    },
    {
      "table": "api_keys",
      "columns": [
        {"name": "id", "type": "INTEGER", "primary": true, "auto": true},
        {"name": "team_id", "type": "INTEGER", "required": true, "references": "teams.id"},
        {"name": "name", "type": "TEXT", "required": true},
        {"name": "key_hash", "type": "TEXT", "required": true, "unique": true},
        {"name": "last_used_at", "type": "DATETIME"},
        {"name": "active", "type": "BOOLEAN", "default": true}
      ]
    }
  ],
  "routes": [
    {"path": "/teams", "method": "POST", "description": "Create a team", "script": "create_team", "response_type": "object"},
    {"path": "/teams", "method": "GET", "description": "List my teams", "script": "my_teams", "response_type": "array"},
    {"path": "/teams/:id", "method": "GET", "description": "Get team details", "script": "get_team", "response_type": "object"},
    {"path": "/teams/:id", "method": "PUT", "description": "Update team", "script": "update_team", "response_type": "object"},
    {"path": "/teams/:id/members", "method": "GET", "description": "List team members", "script": "list_members", "response_type": "array"},
    {"path": "/teams/:id/members", "method": "POST", "description": "Invite member", "script": "invite_member", "response_type": "object"},
    {"path": "/teams/:id/members/:memberId", "method": "DELETE", "description": "Remove member", "script": "remove_member", "response_type": "object"},
    {"path": "/plans", "method": "GET", "description": "List available plans", "script": "list_plans", "response_type": "array"},
    {"path": "/teams/:id/subscription", "method": "GET", "description": "Get team subscription", "script": "get_subscription", "response_type": "object"},
    {"path": "/teams/:id/subscribe", "method": "POST", "description": "Subscribe to plan", "script": "subscribe", "response_type": "object"},
    {"path": "/teams/:id/invoices", "method": "GET", "description": "List team invoices", "script": "list_invoices", "response_type": "array"},
    {"path": "/teams/:id/api-keys", "method": "GET", "description": "List API keys", "script": "list_api_keys", "response_type": "array"},
    {"path": "/teams/:id/api-keys", "method": "POST", "description": "Create API key", "script": "create_api_key", "response_type": "object"},
    {"path": "/admin/teams", "method": "GET", "description": "Admin: list all teams", "script": "admin_list_teams", "response_type": "array"},
    {"path": "/admin/subscriptions", "method": "GET", "description": "Admin: list subscriptions", "script": "admin_list_subs", "response_type": "array"},
    {"path": "/admin/dashboard", "method": "GET", "description": "Admin dashboard", "script": "admin_dashboard", "response_type": "object"}
  ],
  "scripts": [
    {"name": "create_team", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nbody := request.body()\nbody.owner_id = me.user_id\nteam := db.insert(\"teams\", body)\ndb.insert(\"team_members\", {team_id: team.id, user_id: me.user_id, role: \"owner\"})\nresponse.json(team, 201)"},
    {"name": "my_teams", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nresult := db.query(\"SELECT t.* FROM teams t JOIN team_members tm ON t.id = tm.team_id WHERE tm.user_id = ? AND t.deleted_at IS NULL\", [me.user_id])\nresponse.json(result)"},
    {"name": "get_team", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nid := request.param(\"id\")\nteam := db.query_one(\"SELECT * FROM teams WHERE id = ? AND deleted_at IS NULL\", [id])\nif team == undefined {\n  response.fail(404, \"team not found\")\n}\nresponse.json(team)"},
    {"name": "update_team", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nresult := db.update(\"teams\", request.param(\"id\"), request.body())\nresponse.json(result)"},
    {"name": "list_members", "code": "id := request.param(\"id\")\nresult := db.query(\"SELECT tm.*, u.name, u.email FROM team_members tm JOIN users u ON tm.user_id = u.id WHERE tm.team_id = ? AND tm.deleted_at IS NULL\", [id])\nresponse.json(result)"},
    {"name": "invite_member", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nbody := request.body()\nbody.team_id = request.param(\"id\")\nresult := db.insert(\"team_members\", body)\nresponse.json(result, 201)"},
    {"name": "remove_member", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\ndb.delete(\"team_members\", request.param(\"memberId\"))\nresponse.json({\"removed\": true})"},
    {"name": "list_plans", "code": "result := db.query(\"SELECT * FROM plans WHERE active = 1 AND deleted_at IS NULL\", [])\nresponse.json(result)"},
    {"name": "get_subscription", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nid := request.param(\"id\")\nsub := db.query_one(\"SELECT s.*, p.name as plan_name FROM subscriptions s JOIN plans p ON s.plan_id = p.id WHERE s.team_id = ? AND s.deleted_at IS NULL\", [id])\nresponse.json(sub)"},
    {"name": "subscribe", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nbody := request.body()\nbody.team_id = request.param(\"id\")\nresult := db.insert(\"subscriptions\", body)\nresponse.json(result, 201)"},
    {"name": "list_invoices", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nid := request.param(\"id\")\nresult := db.query(\"SELECT i.* FROM invoices i JOIN subscriptions s ON i.subscription_id = s.id WHERE s.team_id = ? AND i.deleted_at IS NULL\", [id])\nresponse.json(result)"},
    {"name": "list_api_keys", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nid := request.param(\"id\")\nresult := db.query(\"SELECT id, team_id, name, active, last_used_at, created_at FROM api_keys WHERE team_id = ? AND deleted_at IS NULL\", [id])\nresponse.json(result)"},
    {"name": "create_api_key", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nbody := request.body()\nbody.team_id = request.param(\"id\")\nkey := crypto.uuid()\nbody.key_hash = crypto.hash(key)\nresult := db.insert(\"api_keys\", body)\nresponse.json({id: result.id, key: key, name: result.name}, 201)"},
    {"name": "admin_list_teams", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nresult := db.query(\"SELECT * FROM teams WHERE deleted_at IS NULL\", [])\nresponse.json(result)"},
    {"name": "admin_list_subs", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nresult := db.query(\"SELECT s.*, t.name as team_name, p.name as plan_name FROM subscriptions s JOIN teams t ON s.team_id = t.id JOIN plans p ON s.plan_id = p.id WHERE s.deleted_at IS NULL\", [])\nresponse.json(result)"},
    {"name": "admin_dashboard", "code": "me := request.auth()\nif me == undefined {\n  response.fail(401, \"authentication required\")\n}\nif me.role != \"admin\" {\n  response.fail(403, \"admin only\")\n}\nteams := db.count(\"teams\")\nusers := db.count(\"users\")\nsubs := db.count(\"subscriptions\")\nresponse.json({\"teams\": teams, \"users\": users, \"subscriptions\": subs})"}
  ],
  "seeds": [
    {"table": "users", "rows": [
      {"email": "admin@saas.com", "password_hash": "$2a$10$placeholder", "name": "Platform Admin", "role": "admin"},
      {"email": "owner@startup.com", "password_hash": "$2a$10$placeholder", "name": "Startup Owner", "role": "user"}
    ]},
    {"table": "plans", "rows": [
      {"name": "Free", "slug": "free", "price_monthly": 0, "price_yearly": 0, "max_members": 3, "features": "Basic features", "active": true},
      {"name": "Pro", "slug": "pro", "price_monthly": 29, "price_yearly": 290, "max_members": 20, "features": "All features + priority support", "active": true},
      {"name": "Enterprise", "slug": "enterprise", "price_monthly": 99, "price_yearly": 990, "max_members": 100, "features": "Everything + custom integrations + SLA", "active": true}
    ]}
  ]
}`
