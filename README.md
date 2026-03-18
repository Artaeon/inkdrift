# InkDrift

Simple, self-hosted newsletter service that works with any SMTP provider.

```
  _       _    ____       _  __ _
 | |     | |  |  _ \ _ __(_)/ _| |_
 | | _ _ | | _| | | | '__| | |_| __|
 | || ' \| / / |_| | |  | |  _| |_
 |_||_||_|_\_\____/|_|  |_|_|  \__|
```

## Features

- **Any SMTP provider** — Hostinger, Hetzner, Contabo, Gmail, Outlook, etc.
- **Subscriber management** — lists, import/export CSV, unsubscribe tokens
- **Campaign system** — create, preview, send with progress tracking
- **REST API** — subscribe endpoint for website integration
- **CLI first** — manage everything from your terminal
- **Single binary** — Go + SQLite, no external dependencies
- **FleetDeck ready** — deploy with `fleetdeck deploy`

## Quick Start

```bash
# Build
make build

# Interactive setup
./inkdrift init

# Create a list
./inkdrift list create "My Newsletter"

# Add subscribers
./inkdrift subscriber add user@example.com --list "My Newsletter"

# Import from CSV
./inkdrift subscriber import subscribers.csv --list "My Newsletter"

# Create and send a campaign
./inkdrift campaign create --name "Welcome" --subject "Hello!" --body-file email.html --list "My Newsletter"
./inkdrift campaign send <campaign-id>

# Start API server for website integration
./inkdrift serve
```

## SMTP Configuration

Works with any SMTP provider. Common examples:

| Provider   | Host                     | Port |
|------------|--------------------------|------|
| Hostinger  | smtp.hostinger.com       | 465  |
| Hetzner    | mail.your-server.de      | 587  |
| Contabo    | mail.contabo.de          | 587  |
| Gmail      | smtp.gmail.com           | 587  |
| Outlook    | smtp-mail.outlook.com    | 587  |

## Website Integration

### Next.js / React

```tsx
import { NewsletterSubscribe } from "./subscribe";

export default function Page() {
  return <NewsletterSubscribe apiUrl="https://newsletter.example.com" />;
}
```

See `examples/nextjs/subscribe.tsx` for the full component.

### Any Website (HTML/JS)

```html
<form id="newsletter-form">
  <input type="email" id="email" placeholder="your@email.com" required>
  <button type="submit">Subscribe</button>
</form>

<script>
document.getElementById("newsletter-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  await fetch("https://newsletter.example.com/api/v1/subscribe", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: document.getElementById("email").value }),
  });
});
</script>
```

## API Endpoints

| Method | Endpoint                       | Auth   | Description           |
|--------|-------------------------------|--------|-----------------------|
| POST   | `/api/v1/subscribe`           | Public | Subscribe an email    |
| GET    | `/api/v1/unsubscribe?token=`  | Public | Unsubscribe           |
| GET    | `/api/v1/confirm?token=`      | Public | Confirm subscription  |
| GET    | `/api/v1/lists`               | Admin  | List all lists        |
| POST   | `/api/v1/lists`               | Admin  | Create a list         |
| GET    | `/api/v1/lists/:id/subscribers`| Admin | List subscribers     |
| GET    | `/api/v1/campaigns`           | Admin  | List campaigns        |
| GET    | `/api/v1/stats`               | Admin  | Dashboard stats       |
| GET    | `/health`                     | None   | Health check          |

Admin endpoints require `X-API-Key` header or `Authorization: Bearer <key>`.

## CLI Commands

```
inkdrift init                          # Interactive setup
inkdrift serve                         # Start API server
inkdrift stats                         # Show statistics
inkdrift test-smtp user@example.com    # Test SMTP config

inkdrift list create "Name"            # Create subscriber list
inkdrift list ls                       # Show all lists
inkdrift list delete <id>              # Delete a list

inkdrift subscriber add email          # Add subscriber
inkdrift subscriber ls                 # List subscribers
inkdrift subscriber import file.csv    # Import from CSV
inkdrift subscriber export -o out.csv  # Export to CSV
inkdrift subscriber remove email       # Unsubscribe

inkdrift campaign create               # Create campaign
inkdrift campaign ls                   # List campaigns
inkdrift campaign send <id>            # Send campaign
inkdrift campaign delete <id>          # Delete campaign

inkdrift template create name -f t.html # Create template
inkdrift template ls                    # List templates
```

## Deploy with FleetDeck

```bash
# One-command deploy
fleetdeck deploy . --domain newsletter.example.com

# Set environment variables
fleetdeck env set inkdrift INKDRIFT_SMTP_HOST=smtp.hostinger.com
fleetdeck env set inkdrift INKDRIFT_SMTP_USERNAME=newsletter@example.com
fleetdeck env set inkdrift INKDRIFT_SMTP_PASSWORD=your-password
fleetdeck env set inkdrift INKDRIFT_SMTP_FROM=newsletter@example.com
```

## Docker

```bash
# Build and run
docker compose up -d

# Or build manually
docker build -t inkdrift .
docker run -p 3377:3377 -v inkdrift-data:/data inkdrift
```

## Configuration

Copy `inkdrift.example.toml` to `inkdrift.toml` and edit:

```toml
[smtp]
host = "smtp.hostinger.com"
port = 465
username = "newsletter@example.com"
password = "your-password"
from = "newsletter@example.com"
from_name = "My Newsletter"
tls = true

[api]
port = 3377
api_key = "your-secret-key"
```

Environment variables override config: `INKDRIFT_SMTP_HOST`, `INKDRIFT_SMTP_PASSWORD`, `INKDRIFT_API_KEY`, etc.

## License

MIT
