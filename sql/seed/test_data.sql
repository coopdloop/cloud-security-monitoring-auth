-- Insert test user (replace with your Auth0 user ID)
INSERT INTO users (auth0_id, name, email, mfa_enabled)
VALUES
    ('auth0|your-actual-auth0-id', 'Test User', 'your-email@example.com', false)
ON CONFLICT (auth0_id) DO UPDATE
SET name = EXCLUDED.name,
    email = EXCLUDED.email;

-- Insert some test sessions
INSERT INTO sessions (user_id, device, location, ip_address, user_agent)
SELECT
    u.id,
    'Chrome on MacOS',
    'New York, US',
    '192.168.1.1'::inet,
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)'
FROM users u
WHERE u.auth0_id = 'auth0|your-actual-auth0-id';

-- Insert some test security logs
INSERT INTO security_logs (user_id, event_type, event_description, ip_address, user_agent)
SELECT
    u.id,
    'login',
    'Successful login',
    '192.168.1.1'::inet,
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)'
FROM users u
WHERE u.auth0_id = 'auth0|your-actual-auth0-id';
