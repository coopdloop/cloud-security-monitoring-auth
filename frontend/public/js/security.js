// frontend/public/js/security.js
//
//

// Global error handler
const handleError = async (response) => {
  if (!response.ok) {
    const error = await response.text().catch(() => 'Unknown error');
    throw new Error(error);
  }
  return response;
};

// API wrapper with error handling
const api = {
  async fetch(endpoint, options = {}) {
    const idToken = localStorage.getItem('id_token');
    if (!idToken) {
      throw new Error('No authentication token found');
    }

    const defaultOptions = {
      headers: {
        'Authorization': `Bearer ${idToken}`,
        'Content-Type': 'application/json'
      }
    };

    try {
      const response = await fetch(endpoint, { ...defaultOptions, ...options });
      await handleError(response);
      return response.json();
    } catch (error) {
      console.error(`API Error (${endpoint}):`, error);
      throw error;
    }
  }
};

// Show error message to user
function showError(message, timeout = 5000) {
  const errorContainer = document.getElementById('errorContainer');
  if (!errorContainer) {
    console.error('Error container not found in DOM');
    alert(message);
    return;
  }

  errorContainer.textContent = message;
  errorContainer.classList.remove('hidden');

  setTimeout(() => {
    errorContainer.classList.add('hidden');
  }, timeout);
}

document.addEventListener('DOMContentLoaded', async () => {

  try {
    const user = await AuthState.init();
    if (!user) return;

    console.log('Fetching security ui data...');

    const [sessions, securityLog] = await Promise.all([
      api.fetch('/api/sessions'),
      api.fetch('/api/security-log')
    ]);

    initializeSecurity(user, sessions, securityLog);

  } catch (error) {
    console.error('Error loading security data:', error);
    showError(error.message || 'Failed to load security settings');
    // Redirect to login if authentication error
    if (error.message.includes('authentication') || error.message.includes('401')) {
      setTimeout(() => {
        window.location.href = '/';
      }, 2000);
    }
  }
});

function initializeSecurity(user, sessions, securityLog) {
  try {
    // Update avatar
    const avatarElement = document.getElementById('userAvatar');
    if (avatarElement) {
      avatarElement.src = user.picture || 'https://placekitten.com/100/100';
    }

    // Initialize MFA toggle (without QR code for now)
    const mfaToggle = document.getElementById('mfaToggle');
    if (mfaToggle) {
      mfaToggle.checked = user.mfa_enabled || false;
      mfaToggle.addEventListener('change', handleMFAToggle);
    }

    // Initialize passwordless button visibility
    const passwordlessButton = document.getElementById('setupPasswordless');
    if (passwordlessButton) {
      passwordlessButton.style.display = user.passwordless_enabled ? 'none' : 'block';
    }

    // Display active sessions if we have the element
    const sessionsContainer = document.getElementById('activeSessions');
    if (sessionsContainer && sessions) {
      displaySessions(sessions);
    }

    // Display security log if we have the element
    const securityLogContainer = document.getElementById('securityLog');
    if (securityLogContainer && securityLog) {
      displaySecurityLog(securityLog);
    }
  } catch (error) {
    console.error('Error initializing security UI:', error);
    showError('Failed to initialize security settings');
  }
}

// Updated MFA toggle handler (simplified without QR code)
async function handleMFAToggle(event) {
  try {
    if (event.target.checked) {
      const response = await api.fetch('/api/setup-mfa', { method: 'POST' });
      showSuccess('MFA settings updated successfully');
    } else {
      const confirmed = confirm('Are you sure you want to disable MFA? This will make your account less secure.');
      if (!confirmed) {
        event.target.checked = true;
        return;
      }

      await api.fetch('/api/disable-mfa', { method: 'POST' });
      showSuccess('Multi-factor authentication has been disabled');
    }
  } catch (error) {
    event.target.checked = !event.target.checked;
    showError('Failed to update MFA settings: ' + error.message);
  }
}

// async function handleMFAToggle(event) {
//   try {
//     if (event.target.checked) {
//       const response = await api.fetch('/api/setup-mfa', { method: 'POST' });
//       if (response.qr_code) {
//         document.getElementById('mfaSetupSection').classList.remove('hidden');
//         document.getElementById('qrCode').src = response.qr_code;
//         // Store ticket ID if needed
//         if (response.ticket_id) {
//           document.getElementById('mfaSetupSection').dataset.ticketId = response.ticket_id;
//         }
//       }
//     } else {
//       const confirmed = confirm('Are you sure you want to disable MFA? This will make your account less secure.');
//       if (!confirmed) {
//         event.target.checked = true;
//         return;
//       }
//
//       await api.fetch('/api/disable-mfa', { method: 'POST' });
//       document.getElementById('mfaSetupSection').classList.add('hidden');
//       showSuccess('Multi-factor authentication has been disabled');
//     }
//   } catch (error) {
//     event.target.checked = !event.target.checked;
//     showError('Failed to update MFA settings: ' + error.message);
//   }
// }

async function verifyMFA() {
  const codeInput = document.getElementById('mfaCode');
  const code = codeInput.value.trim();

  if (!code) {
    showError('Please enter the verification code');
    return;
  }

  try {
    await api.fetch('/api/verify-mfa', {
      method: 'POST',
      body: JSON.stringify({ code })
    });

    document.getElementById('mfaSetupSection').classList.add('hidden');
    showError('MFA enabled successfully', 3000);
    codeInput.value = '';
  } catch (error) {
    showError('Failed to verify MFA code: ' + error.message);
  }
}

async function revokeSession(sessionId) {
  if (!confirm('Are you sure you want to revoke this session?')) {
    return;
  }

  try {
    await api.fetch(`/api/sessions/${sessionId}`, { method: 'DELETE' });
    const sessions = await api.fetch('/api/sessions');
    displaySessions(sessions);
    showError('Session revoked successfully', 3000);
  } catch (error) {
    showError('Failed to revoke session: ' + error.message);
  }
}

async function setupPasswordless() {
  try {
    const response = await api.fetch('/api/setup-passwordless', { method: 'POST' });

    // Create verification code input modal
    const modal = document.createElement('div');
    modal.className = 'fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center';
    modal.innerHTML = `
            <div class="bg-white p-6 rounded-lg shadow-xl max-w-md w-full">
                <h3 class="text-lg font-bold mb-4">Enter Verification Code</h3>
                <p class="mb-4 text-gray-600">Please check your email (${response.email}) for a verification code.</p>
                <div class="mb-4">
                    <input type="text" id="verificationCode"
                           class="w-full p-2 border rounded focus:border-blue-500 focus:outline-none"
                           placeholder="Enter code">
                </div>
                <div class="flex justify-end space-x-2">
                    <button class="px-4 py-2 bg-gray-200 rounded hover:bg-gray-300" onclick="closePasswordlessModal()">
                        Cancel
                    </button>
                    <button class="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600" onclick="verifyPasswordlessCode()">
                        Verify
                    </button>
                </div>
            </div>
        `;

    document.body.appendChild(modal);
    document.getElementById('verificationCode').focus();

  } catch (error) {
    showError('Failed to setup passwordless authentication: ' + error.message);
  }
}

function closePasswordlessModal() {
  const modal = document.querySelector('.fixed.inset-0');
  if (modal) {
    modal.remove();
  }
}

async function verifyPasswordlessCode() {
  const code = document.getElementById('verificationCode').value.trim();
  if (!code) {
    showError('Please enter the verification code');
    return;
  }

  try {
    const response = await api.fetch('/api/verify-passwordless', {
      method: 'POST',
      body: JSON.stringify({ code })
    });

    closePasswordlessModal();
    showSuccess(response.message);

    // Refresh the page after successful verification
    setTimeout(() => {
      window.location.reload();
    }, 1500);
  } catch (error) {
    showError('Failed to verify code: ' + error.message);
  }
}

// Helper function to create message container
function createMessageContainer() {
  const container = document.createElement('div');
  container.id = 'messageContainer';
  container.className = 'hidden fixed top-4 right-4 px-4 py-2 rounded shadow';
  document.body.appendChild(container);
  return container;
}

// Show error message to user
function showError(message, timeout = 5000) {
  const container = document.getElementById('messageContainer') || createMessageContainer();
  container.textContent = message;
  container.className = 'fixed top-4 right-4 bg-red-500 text-white px-4 py-2 rounded shadow';
  setTimeout(() => {
    container.className = 'hidden';
  }, timeout);
}

// Show success message to user
function showSuccess(message, timeout = 5000) {
  const container = document.getElementById('messageContainer') || createMessageContainer();
  container.textContent = message;
  container.className = 'fixed top-4 right-4 bg-green-500 text-white px-4 py-2 rounded shadow';
  setTimeout(() => {
    container.className = 'hidden';
  }, timeout);
}

function displaySessions(sessions) {
  const container = document.getElementById('activeSessions');
  container.innerHTML = sessions.map(session => `
        <div class="flex justify-between items-center p-4 bg-base-200 rounded-lg">
            <div>
                <div class="font-bold">${session.device}</div>
                <div class="text-sm opacity-70">
                    ${session.location} • Last active ${new Date(session.last_active).toLocaleDateString()}
                </div>
            </div>
            <button class="btn btn-sm btn-error" onclick="revokeSession('${session.id}')">
                Revoke
            </button>
        </div>
    `).join('') || '<p class="text-center opacity-70">No active sessions</p>';
}

function displaySecurityLog(logs) {
  const container = document.getElementById('securityLog');
  container.innerHTML = logs.map(log => `
        <div class="flex items-center space-x-4 p-2">
            <div class="w-2 h-2 rounded-full ${getEventColor(log.type)}"></div>
            <div class="flex-grow">
                <div class="font-medium">${log.event}</div>
                <div class="text-sm opacity-70">
                    ${new Date(log.timestamp).toLocaleString()} • ${log.ip_address}
                </div>
            </div>
        </div>
    `).join('') || '<p class="text-center opacity-70">No security events</p>';
}

function getEventColor(type) {
  const colors = {
    'login': 'bg-success',
    'logout': 'bg-info',
    'mfa_enabled': 'bg-success',
    'mfa_disabled': 'bg-warning',
    'password_changed': 'bg-warning',
    'suspicious_activity': 'bg-error'
  };
  return colors[type] || 'bg-base-300';
}

async function revokeSession(sessionId) {
  const idToken = localStorage.getItem('id_token');
  try {
    const response = await fetch(`/api/sessions/${sessionId}`, {
      method: 'DELETE',
      headers: {
        'Authorization': `Bearer ${idToken}`,
        'Content-Type': 'application/json'
      }
    });

    if (!response.ok) throw new Error('Failed to revoke session');

    // Refresh the sessions list
    const sessionsResponse = await fetch('/api/sessions', {
      headers: {
        'Authorization': `Bearer ${idToken}`,
        'Content-Type': 'application/json'
      }
    });

    if (sessionsResponse.ok) {
      const sessions = await sessionsResponse.json();
      displaySessions(sessions);
    }
  } catch (error) {
    console.error('Error revoking session:', error);
    alert('Failed to revoke session');
  }
}
// Utility functions
function logout() {
  localStorage.removeItem('access_token');
  localStorage.removeItem('id_token');
  window.location.href = '/logout';
}
