// frontend/public/js/profile.js
document.addEventListener('DOMContentLoaded', async () => {
  try {
    const user = await AuthState.init();
    if (!user) return;
    initializeProfile(user);
  } catch (error) {
    console.error('Error fetching user data:', error);
    window.location.href = '/';
  }
});

function initializeProfile(user) {
  document.getElementById('userAvatar').src = user.picture;
  document.getElementById('profilePicture').src = user.picture;
  document.getElementById('displayName').value = user.name;
  document.getElementById('email').value = user.email;

  // Handle profile form submission
  document.getElementById('profileForm').addEventListener('submit', async (e) => {
    e.preventDefault();

    try {
      const idToken = localStorage.getItem('id_token');
      const formData = new FormData();
      formData.append('name', document.getElementById('displayName').value);

      const pictureFile = document.getElementById('pictureUpload').files[0];
      if (pictureFile) {
        formData.append('picture', pictureFile);
      }

      const response = await fetch('/api/user/profile', {
        method: 'PUT',
        body: formData,
        headers: {
          'Authorization': `Bearer ${idToken}`
        }
      });

      if (!response.ok) throw new Error('Failed to update profile');

      // Show success message
      alert('Profile updated successfully');
    } catch (error) {
      console.error('Error updating profile:', error);
      alert('Failed to update profile');
    }
  });
}

function logout() {
  // Clear both tokens
  localStorage.removeItem('access_token');
  localStorage.removeItem('id_token');
  window.location.href = '/logout';
}
