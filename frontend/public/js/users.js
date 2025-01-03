// frontend/public/js/users.js

let currentUserId = null;
let availableRoles = [];

async function initializePage() {
  try {
    // Initialize auth state and get user data
    const userData = await AuthState.init();
    if (!userData) {
      return; // AuthState will handle showing the unauthorized screen
    }

    await loadUsers();
    await loadUserClaims();
  } catch (error) {
    console.error('Failed to initialize page:', error);
    AuthState.showUnauthorized();
  }
}

async function loadUsers() {
  try {
    const idToken = localStorage.getItem('id_token');
    const response = await fetch('/api/users/roles', {
      headers: {
        'Authorization': `Bearer ${idToken}`
      }
    });

    if (!response.ok) {
      if (response.status === 401 || response.status === 403) {
        AuthState.showUnauthorized();
        return;
      }
      throw new Error('Failed to fetch users');
    }

    const users = await response.json();
    console.log('Users data:', users);
    displayUsers(users);

    // Also load available roles
    await loadAvailableRoles();
  } catch (error) {
    console.error('Error loading users:', error);
    alert('Failed to load users. Please try again.');
  }
}

async function loadAvailableRoles() {
  try {
    const idToken = localStorage.getItem('id_token');
    const response = await fetch('/api/roles', {
      headers: {
        'Authorization': `Bearer ${idToken}`
      }
    });

    if (!response.ok) {
      throw new Error('Failed to fetch roles');
    }

    availableRoles = await response.json();
  } catch (error) {
    console.error('Error loading roles:', error);
  }
}

function parseRoles(roles) {
  try {
    if (typeof (roles) === 'string') {
      // If it's a comma-separated string, split it
      return roles.split(',').filter(role => role.trim() !== '');
    }
    // If it's already an array, return it
    return roles || [];
  } catch (error) {
    console.error('Error parsing roles:', error, roles);
    return [];
  }
}

function displayUsers(users) {
  console.log('Displaying users:', users);
  const tbody = document.getElementById('usersTableBody');
  tbody.innerHTML = '';

  users.forEach(user => {
    const userRoles = parseRoles(user.roles);
    console.log(`User ${user.id} roles:`, userRoles);

    const tr = document.createElement('tr');
    tr.innerHTML = `
            <td>
                <div class="flex items-center space-x-3">
                    <div class="avatar placeholder">
                        <div class="bg-neutral text-neutral-content rounded-full w-8">
                            <span class="text-xs">${user.name?.substring(0, 2).toUpperCase() || 'U'}</span>
                        </div>
                    </div>
                    <div>
                        <div class="font-bold">${user.name || 'Unknown'}</div>
                    </div>
                </div>
            </td>
            <td>
                <div class="flex items-center space-x-3">
                        <div class="text-sm opacity-50">${user.email}</div>
                </div>
            </td>
            <td>
                <div class="flex flex-wrap gap-1">
                    ${userRoles.length > 0 ?
        userRoles.map(role =>
          `<span class="badge badge-primary badge-sm">${role}</span>`
        ).join('')
        : 'No roles'}
                </div>
            </td>
            <td class="text-right">
                <div class="flex justify-end gap-2">
                    <button onclick="editUserRoles(${user.id})" class="btn btn-sm btn-ghost">
                        Edit Roles
                    </button>
                    ${userRoles.includes('admin') ?
        `<button onclick="setUserAdmin(${user.id}, false)" class="btn btn-sm btn-warning">
                            Remove Admin
                        </button>` :
        `<button onclick="setUserAdmin(${user.id}, true)" class="btn btn-sm btn-primary">
                            Make Admin
                        </button>`
      }
                </div>
            </td>
        `;
    tbody.appendChild(tr);
  });
}

function editUserRoles(userId) {
  currentUserId = userId;
  const modal = document.getElementById('editRolesModal');
  const checkboxesContainer = document.getElementById('rolesCheckboxes');

  // Clear existing checkboxes
  checkboxesContainer.innerHTML = '';

  // Get current user's roles
  const idToken = localStorage.getItem('id_token');
  fetch(`/api/users/roles/${userId}`, {
    headers: {
      'Authorization': `Bearer ${idToken}`
    }
  })
    .then(response => {
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      return response.json();
    })
    .then(userRoles => {
      console.log('User roles for edit:', userRoles);
      // Create checkbox for each available role
      availableRoles.forEach(role => {
        const isChecked = userRoles.includes(role.name);
        const div = document.createElement('div');
        div.className = 'form-control';
        div.innerHTML = `
                <label class="label cursor-pointer">
                    <span class="label-text">${role.name}</span>
                    <input type="checkbox" class="checkbox"
                           value="${role.name}"
                           ${isChecked ? 'checked' : ''}>
                </label>
            `;
        checkboxesContainer.appendChild(div);
      });

      // Show modal using DaisyUI modal
      modal.classList.add('modal-open');
    })
    .catch(error => {
      console.error('Error fetching user roles:', error);
      alert('Failed to load user roles');
    });
}

function closeEditModal() {
  const modal = document.getElementById('editRolesModal');
  modal.classList.remove('modal-open');
  currentUserId = null;
}

async function saveUserRoles() {
  if (!currentUserId) return;

  const checkboxes = document.querySelectorAll('#rolesCheckboxes input[type="checkbox"]');
  const selectedRoles = Array.from(checkboxes)
    .filter(cb => cb.checked)
    .map(cb => cb.value);

  try {
    const idToken = localStorage.getItem('id_token');
    const response = await fetch('/api/users/roles', {
      method: 'PUT',
      headers: {
        'Authorization': `Bearer ${idToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        user_id: currentUserId,
        roles: selectedRoles
      })
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Failed to update roles: ${errorText}`);
    }

    // Close modal and reload users
    closeEditModal();
    await loadUsers();
  } catch (error) {
    console.error('Error saving roles:', error);
    alert(error.message || 'Failed to save roles. Please try again.');
  }
}

async function setUserAdmin(userId, isAdmin) {
  try {
    const idToken = localStorage.getItem('id_token');
    const response = await fetch('/api/users/set-admin', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${idToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        user_id: userId,
        is_admin: isAdmin
      })
    });

    if (!response.ok) {
      // Try to get error details from response
      const errorText = await response.text();
      console.error('Server error:', errorText);
      throw new Error(`Failed to update admin status: ${errorText}`);
    }

    // Reload users to show updated roles
    await loadUsers();
  } catch (error) {
    console.error('Error setting admin status:', error);
    alert(error.message || 'Failed to update admin status. Please try again.');
  }
}

function showCreateUserModal() {
  const modal = document.getElementById('createUserModal');
  const form = document.getElementById('createUserForm');

  // Reset form
  form.reset();

  // Populate role checkboxes
  const rolesContainer = document.getElementById('createUserRoles');
  rolesContainer.innerHTML = '';

  availableRoles.forEach(role => {
    const div = document.createElement('div');
    div.className = 'form-control';
    div.innerHTML = `
            <label class="label cursor-pointer">
                <span class="label-text">${role.name}</span>
                <input type="checkbox" class="checkbox"
                       value="${role.name}">
            </label>
        `;
    rolesContainer.appendChild(div);
  });

  // Show modal
  modal.classList.add('modal-open');
}

async function createUser() {
  const form = document.getElementById('createUserForm');
  const name = document.getElementById('newUserName').value;
  const email = document.getElementById('newUserEmail').value;

  // Collect selected roles
  const checkboxes = document.querySelectorAll('#createUserRoles input[type="checkbox"]:checked');
  const selectedRoles = Array.from(checkboxes).map(cb => cb.value);

  try {
    const idToken = localStorage.getItem('id_token');
    const response = await fetch('/api/users', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${idToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        name: name,
        email: email,
        // Optional fields
        auth0_id: '', // Backend will generate if empty
        picture: '', // Optional picture URL
        roles: selectedRoles
      })
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Failed to create user: ${errorText}`);
    }

    // Close modal and reload users
    closeCreateUserModal();
    await loadUsers();
  } catch (error) {
    console.error('Error creating user:', error);
    alert(error.message || 'Failed to create user. Please try again.');
  }
}

function closeCreateUserModal() {
  const modal = document.getElementById('createUserModal');
  modal.classList.remove('modal-open');
}


async function loadUserClaims() {
  try {
    const idToken = localStorage.getItem('id_token');
    const response = await fetch('/api/user/claims', {
      headers: {
        'Authorization': `Bearer ${idToken}`
      }
    });

    if (!response.ok) {
      throw new Error('Failed to fetch user claims');
    }

    const claims = await response.json();
    renderUserClaims(claims);
  } catch (error) {
    console.error('Error loading user claims:', error);
  }
}

function renderUserClaims(claims) {
  const claimsContainer = document.getElementById('userClaimsContainer');
  if (!claimsContainer) return;

  // Clear existing claims
  claimsContainer.innerHTML = '';

  // Render each claim
  claims.forEach(claim => {
    const claimElement = document.createElement('div');
    claimElement.classList.add('claim-item');

    switch (claim.key) {
      case 'department':
        const dept = JSON.parse(claim.value);
        claimElement.innerHTML = `
                    <strong>Department:</strong> ${dept.name}
                    <button onclick="editDepartmentClaim()">Edit</button>
                `;
        break;
      case 'permissions':
        const perms = JSON.parse(claim.value);
        claimElement.innerHTML = `
                    <strong>Permissions:</strong> ${perms.join(', ')}
                    <button onclick="editPermissionsClaim()">Edit</button>
                `;
        break;
      case 'project_access':
        const projects = JSON.parse(claim.value);
        claimElement.innerHTML = `
                    <strong>Project Access:</strong>
                    <ul>
                        ${projects.map(p => `<li>${p.id} - ${p.role}</li>`).join('')}
                    </ul>
                    <button onclick="editProjectAccessClaim()">Edit</button>
                `;
        break;
    }

    claimsContainer.appendChild(claimElement);
  });
}

function editDepartmentClaim() {
  const newDept = prompt('Enter department name:');
  if (newDept) {
    updateUserClaims([{
      key: 'department',
      value: JSON.stringify({ name: newDept })
    }]);
  }
}

function editPermissionsClaim() {
  const permissionsInput = prompt('Enter permissions (comma-separated):');
  if (permissionsInput) {
    const permissions = permissionsInput.split(',').map(p => p.trim());
    updateUserClaims([{
      key: 'permissions',
      value: JSON.stringify(permissions)
    }]);
  }
}

async function updateUserClaims(claims) {
  try {
    const idToken = localStorage.getItem('id_token');
    const response = await fetch('/api/user/claims', {
      method: 'PUT',
      headers: {
        'Authorization': `Bearer ${idToken}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(claims)
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`Failed to update claims: ${errorText}`);
    }

    // Reload claims
    await loadUserClaims();
  } catch (error) {
    console.error('Error updating claims:', error);
    alert(error.message);
  }
}

function logout() {
  // Clear both tokens
  localStorage.removeItem('access_token');
  localStorage.removeItem('id_token');
  window.location.href = '/logout';
}


// Initialize page when DOM is loaded
document.addEventListener('DOMContentLoaded', initializePage);
