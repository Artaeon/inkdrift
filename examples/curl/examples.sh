#!/bin/bash
# InkDrift API Examples using curl
# Change the base URL to your InkDrift instance

API="http://localhost:3377"
API_KEY="your-api-key"  # Set if you configured one

# Subscribe a new email
curl -X POST "$API/api/v1/subscribe" \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com", "name": "John Doe", "list": "My Newsletter"}'

# List all subscriber lists (admin)
curl "$API/api/v1/lists" \
  -H "X-API-Key: $API_KEY"

# Create a new list (admin)
curl -X POST "$API/api/v1/lists" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: $API_KEY" \
  -d '{"name": "Product Updates", "description": "Monthly product updates"}'

# Get subscribers for a list (admin)
curl "$API/api/v1/lists/LIST_ID/subscribers" \
  -H "X-API-Key: $API_KEY"

# Get stats (admin)
curl "$API/api/v1/stats" \
  -H "X-API-Key: $API_KEY"

# Health check
curl "$API/health"
