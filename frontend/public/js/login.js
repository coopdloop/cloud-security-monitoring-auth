// frontend/public/js/login.js

async function verifyCode() {
    const code = document.getElementById('code').value.trim();
    const email = document.getElementById('email').value.trim(); // Make sure to keep the email field in the DOM

    if (!code) {
        alert('Please enter the verification code');
        return;
    }

    try {
        const response = await fetch('/passwordless/verify', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                code,
                email  // Include the email in the verification request
            })
        });

        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(errorText || 'Invalid verification code');
        }

        const data = await response.json();

        // Store tokens
        localStorage.setItem('access_token', data.access_token);
        localStorage.setItem('id_token', data.id_token);

        // Redirect to dashboard on success
        window.location.href = '/dashboard';
    } catch (error) {
        alert('Error: ' + error.message);
    }
}
