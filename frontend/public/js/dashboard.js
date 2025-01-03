// frontend/public/js/dashboard.js
async function init() {
  try {

    const user = await AuthState.init();
    if (!user) return;

    updateUI(user);

  } catch (error) {
    console.error('Error fetching user data:', error);
  }
}

function updateUI(user) {
  // Update avatar
  document.getElementById('userAvatar').src = user.picture || 'https://placekitten.com/100/100';

  // Update user info
  document.getElementById('userInfo').innerHTML = `
        <div class="grid grid-cols-2 gap-4">
            <div class="font-bold">Name:</div>
            <div>${user.name}</div>
            <div class="font-bold">Email:</div>
            <div>${user.email}</div>
            <div class="font-bold">Nickname:</div>
            <div>${user.nickname || 'Not set'}</div>
        </div>
    `;

  // Update auth info
  document.getElementById('authInfo').innerHTML = `
        <div class="grid grid-cols-2 gap-4">
            <div class="font-bold">User ID:</div>
            <div>${user.sub}</div>
            <div class="font-bold">Email Verified:</div>
            <div>${user.email_verified ?
      '<span class="text-success">Yes</span>' :
      '<span class="text-error">No</span>'}</div>
            <div class="font-bold">MFA Enabled:</div>
            <div>${user.mfa_enabled ?
      '<span class="text-success">Yes</span>' :
      '<span class="text-error">No</span>'}</div>
            <div class="font-bold">Last Updated:</div>
            <div>${new Date(user.updated_at).toLocaleDateString()}</div>
        </div>
    `;

  // Update roles & permissions
  document.getElementById('rolesInfo').innerHTML = `
        <div class="space-y-4">
            <div>
                <h3 class="font-bold mb-2">Roles:</h3>
                <ul class="list-disc list-inside">
                    ${user.roles?.length ?
      user.roles.map(role => `<li>${role}</li>`).join('') :
      '<li class="text-gray-500">No roles assigned</li>'}
                </ul>
            </div>
            <div>
                <h3 class="font-bold mb-2">Permissions:</h3>
                <ul class="list-disc list-inside">
                    ${user.permissions?.length ?
      user.permissions.map(perm => `<li>${perm}</li>`).join('') :
      '<li class="text-gray-500">No permissions assigned</li>'}
                </ul>
            </div>
        </div>
    `;

  // Update MFA toggle
  document.getElementById('mfaToggle').checked = user.mfa_enabled;
}

async function setupMFA() {
  try {
    const response = await fetch('/api/setup-mfa', {
      method: 'POST',
      credentials: 'include'
    });

    if (!response.ok) throw new Error('Failed to setup MFA');

    const data = await response.json();
    // Handle MFA setup - this might open a new window or show QR code
    window.location.href = data.setup_uri;
  } catch (error) {
    console.error('Error setting up MFA:', error);
  }
}

async function setupPasswordless() {
  try {
    const response = await fetch('/api/setup-passwordless', {
      method: 'POST',
      // credentials: 'include'
    });

    if (!response.ok) throw new Error('Failed to setup passwordless');

    const data = await response.json();
    // Redirect to Auth0 passwordless setup
    window.location.href = data.setup_uri;
  } catch (error) {
    console.error('Error setting up passwordless:', error);
  }
}

async function goToSecuritySettings() {
  window.location.href = '/security.html'
}

// Add this function to handle the callback
async function handleCallback(code) {
  try {
    const response = await fetch(`/callback?code=${code}`);
    if (!response.ok) throw new Error('Failed to exchange code for tokens');

    const tokens = await response.json();

    // Store tokens
    localStorage.setItem('access_token', tokens.access_token);
    localStorage.setItem('id_token', tokens.id_token);

    // Redirect to dashboard
    window.location.href = '/dashboard.html';
  } catch (error) {
    console.error('Error handling callback:', error);
    window.location.href = '/';
  }
}

function logout() {
  // Clear both tokens
  localStorage.removeItem('access_token');
  localStorage.removeItem('id_token');
  window.location.href = '/logout';
}

init()
