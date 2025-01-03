// frontend/public/js/auth-state.js

const AuthState = {
  // HTML templates for different states
  templates: {
    loading: `
            <div id="loadingScreen" class="fixed inset-0 bg-white z-50 flex items-center justify-center">
                <div class="text-center">
                    <div class="animate-spin rounded-full h-32 w-32 border-b-2 border-gray-900"></div>
                    <p class="mt-4 text-xl">Loading...</p>
                </div>
            </div>
        `,
    unauthorized: `
            <div id="unauthorizedScreen" class="fixed inset-0 bg-white z-50 flex items-center justify-center">
                <div class="text-center">
                    <h2 class="text-2xl font-bold mb-4">Unauthorized Access</h2>
                    <p class="mb-4">Please log in to access this page.</p>
                    <button onclick="window.location.href='/'" class="bg-blue-500 text-white px-4 py-2 rounded">
                        Go to Login
                    </button>
                </div>
            </div>
        `
  },

  // Track current state
  currentState: 'loading',

  // Initialize auth state and screens
  init(options = {}) {
    // Add screens to DOM if they don't exist
    if (!document.getElementById('loadingScreen')) {
      const loadingDiv = document.createElement('div');
      loadingDiv.innerHTML = this.templates.loading;
      document.body.appendChild(loadingDiv.firstElementChild);
    }

    if (!document.getElementById('unauthorizedScreen')) {
      const unauthorizedDiv = document.createElement('div');
      unauthorizedDiv.innerHTML = this.templates.unauthorized;
      document.body.appendChild(unauthorizedDiv.firstElementChild);
    }

    // Hide main content initially
    const mainContent = document.getElementById('mainContent');
    if (mainContent) {
      mainContent.classList.add('hidden');
    }

    // Show loading screen
    this.showLoading();

    // Check authentication
    return this.checkAuth(options);
  },

  // Check authentication status
  async checkAuth(options = {}) {
    const idToken = localStorage.getItem('id_token');
    if (!idToken) {
      this.showUnauthorized();
      return false;
    }

    try {
      // Verify token is valid by making a request to /api/user
      const response = await fetch('/api/user', {
        headers: {
          'Authorization': `Bearer ${idToken}`
        }
      });

      if (!response.ok) {
        throw new Error('Invalid token');
      }

      const userData = await response.json();
      this.showContent();
      return userData;

    } catch (error) {
      console.error('Auth check failed:', error);
      this.showUnauthorized();
      return false;
    }
  },

  // State management methods
  showLoading() {
    this.currentState = 'loading';
    document.getElementById('loadingScreen')?.classList.remove('hidden');
    document.getElementById('unauthorizedScreen')?.classList.add('hidden');
    document.getElementById('mainContent')?.classList.add('hidden');
  },

  showUnauthorized() {
    this.currentState = 'unauthorized';
    document.getElementById('loadingScreen')?.classList.add('hidden');
    document.getElementById('unauthorizedScreen')?.classList.remove('hidden');
    document.getElementById('mainContent')?.classList.add('hidden');
  },

  showContent() {
    this.currentState = 'content';
    document.getElementById('loadingScreen')?.classList.add('hidden');
    document.getElementById('unauthorizedScreen')?.classList.add('hidden');
    document.getElementById('mainContent')?.classList.remove('hidden');
  },

  // Utility methods
  redirectToLogin() {
    window.location.href = '/';
  },

  logout() {
    localStorage.removeItem('access_token');
    localStorage.removeItem('id_token');
    this.redirectToLogin();
  }
};
