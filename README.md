# Go Security Dashboard

A full-stack security management application built with Go and vanilla JavaScript, featuring Auth0 integration, session management, and security logging.

## Features

- **Authentication & Authorization**
  - Auth0 integration for secure user authentication
  - OAuth2 flow with PKCE support
  - Multi-factor authentication (MFA) support
  - Passwordless authentication options

- **Session Management**
  - Active session tracking
  - Device and location tracking
  - Session revocation capabilities
  - IP address logging

- **Security Logging**
  - Comprehensive security event tracking
  - Login attempt monitoring
  - MFA status changes
  - Suspicious activity detection

- **User Management**
  - User profile management
  - MFA enrollment and management
  - Session history viewing

## Tech Stack

### Backend
- **Go** - Primary backend language
- **Chi Router** - HTTP routing
- **SQLC** - Type-safe SQL query generation
- **PostgreSQL** - Primary database
- **Auth0** - Authentication provider
- **zerolog** - Structured logging

### Frontend
- **Vanilla JavaScript** - No framework dependencies
- **TailwindCSS** - Styling
- **HTML5** - Static pages

### Development Tools
- **Docker** - Containerization
- **Docker Compose** - Multi-container orchestration
- **Adminer** - Database management
- **OpenAPI** - API documentation and code generation

## Project Structure

```
.
├── api/
│   └── openapi.yaml      # OpenAPI specification
├── cmd/
│   └── server/
│       └── main.go       # Application entry point
├── db/
│   └── migrations/       # Database migrations
├── frontend/
│   └── public/
│       ├── js/          # Frontend JavaScript
│       ├── *.html       # Static pages
│       └── styles.css   # CSS styles
├── internal/
│   ├── api/            # API handlers and middleware
│   ├── auth/           # Auth0 integration
│   ├── db/            # Database operations
│   └── logging/       # Logging configuration
├── sql/
│   ├── queries/       # SQLC queries
│   └── schema/        # Database schema
└── docker-compose.yml # Container configuration
```

## Setup Instructions

### Prerequisites

- Go 1.21 or higher
- Docker and Docker Compose
- Auth0 account
- PostgreSQL (optional for local development)

### Development Environment Setup

1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd <repository-name>
   ```

2. Create environment files:

   Create `dev.env` for local development:
   ```env
   AUTH0_DOMAIN=your-domain.auth0.com
   AUTH0_CLIENT_ID=your-client-id
   AUTH0_CLIENT_SECRET=your-client-secret
   AUTH0_CALLBACK_URL=http://localhost:3000/callback.html
   DATABASE_URL=postgres://postgres:postgres@localhost:5432/myapp?sslmode=disable
   ```

   Create `prod.env` for production:
   ```env
   DATABASE_URL=postgres://postgres:postgres@db:5432/myapp?sslmode=disable
   # Add other production variables
   ```

3. Set up Auth0:
   - Create a new Auth0 application
   - Enable the following features:
     - OAuth2 authentication
     - MFA
     - Passwordless login
   - Add callback URLs:
     - `http://localhost:3000/callback.html`
     - `http://localhost:3000/callback`
   - Grant required Management API permissions

4. Initialize the database:
   ```bash
   # Start the database container
   docker-compose up db -d

   # Apply migrations
   psql -h localhost -U postgres -d myapp -f sql/schema/001_init.sql
   psql -h localhost -U postgres -d myapp -f sql/schema/002_security_tables.sql
   ```

5. Generate code:
   ```bash
   # Generate database code
   sqlc generate

   # Generate API code
   oapi-codegen -package api api/openapi.yaml > internal/api/api.gen.go
   ```

### Running the Application

#### Local Development
```bash
# Start the database
docker-compose up db -d

# Run the application
go run ./cmd/server/main.go
```

#### Docker Environment
```bash
# Build and start all services
docker-compose up --build
```

The application will be available at:
- Main application: http://localhost:3000
- Adminer (database management): http://localhost:8080

## Security Considerations

- Store sensitive environment variables securely
- Never commit .env files to version control
- Regularly rotate Auth0 client secrets
- Monitor security logs for suspicious activity
- Keep dependencies updated
- Use HTTPS in production

## Development Workflow

1. **Database Changes**
   - Add new migrations in `sql/schema/`
   - Update queries in `sql/queries/`
   - Generate new code with `sqlc generate`

2. **API Changes**
   - Update `api/openapi.yaml`
   - Generate new handlers with `oapi-codegen`
   - Implement new handlers in `internal/api/`

3. **Frontend Changes**
   - Update HTML in `frontend/public/`
   - Modify JavaScript in `frontend/public/js/`
   - Test with local development server

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests
5. Submit a pull request

## License

[Your chosen license]

## Support

For issues and questions:
- Submit GitHub issues
- Contact [your contact information]
