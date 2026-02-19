// Settings Store - Configuration and User Management

document.addEventListener('alpine:init', () => {
    Alpine.store('settings', {
        config: {
            organizationName: '',
            publicDomain: '',
            internalPort: 8080,
            publicPort: 8443,
            allowedIPs: '',
            enableAllowlist: false
        },
        publicKeyPreview: 'Loading...',
        users: [],
        newUser: {
            username: '',
            password: ''
        },
        passwordChange: {
            currentPassword: '',
            newPassword: ''
        },
        
        async loadSettings() {
            await Promise.all([
                this.loadConfig(),
                this.loadUsers()
            ]);
        },
        
        async loadConfig() {
            try {
                const config = await window.utils.api.get('/api/settings/config');
                this.config = {
                    organizationName: config.organization_name || '',
                    publicDomain: config.public_domain || '',
                    internalPort: config.internal_api_port || 8080,
                    publicPort: config.public_api_port || 8443,
                    allowedIPs: (config.allowed_ip_addresses || []).join(', '),
                    enableAllowlist: config.enable_ip_allowlist || false
                };
                this.publicKeyPreview = config.public_key_preview || 'No key generated';
            } catch (error) {
                console.error('Failed to load config:', error);
                window.utils.showNotification('Failed to load configuration', 'error');
            }
        },
        
        async saveConfig() {
            const payload = {
                organization_name: this.config.organizationName,
                public_domain: this.config.publicDomain,
                internal_api_port: this.config.internalPort,
                public_api_port: this.config.publicPort,
                allowed_ip_addresses: this.config.allowedIPs.split(',').map(ip => ip.trim()).filter(ip => ip),
                enable_ip_allowlist: this.config.enableAllowlist
            };
            
            try {
                await window.utils.api.put('/api/settings/config', payload);
                window.utils.showNotification('Configuration saved successfully', 'success');
            } catch (error) {
                console.error('Failed to save config:', error);
                window.utils.showNotification('Failed to save configuration', 'error');
            }
        },
        
        async rotateKeys() {
            if (!confirm('Are you sure? This will invalidate existing partner connections until they update your public key.')) {
                return;
            }
            
            try {
                await window.utils.api.post('/api/settings/rotate-keys', {});
                window.utils.showNotification('Keys rotated successfully', 'success');
                this.loadConfig();
            } catch (error) {
                console.error('Failed to rotate keys:', error);
                window.utils.showNotification('Failed to rotate keys', 'error');
            }
        },
        
        async loadUsers() {
            try {
                const data = await window.utils.api.get('/api/settings/users');
                this.users = data.users || [];
            } catch (error) {
                console.error('Failed to load users:', error);
                this.users = [];
            }
        },
        
        async createUser() {
            if (!this.newUser.username || !this.newUser.password) {
                window.utils.showNotification('Username and password required', 'error');
                return;
            }
            
            try {
                await window.utils.api.post('/api/settings/users', this.newUser);
                window.utils.showNotification('User created successfully', 'success');
                this.newUser = {username: '', password: ''};
                this.loadUsers();
            } catch (error) {
                console.error('Failed to create user:', error);
                window.utils.showNotification('Failed to create user', 'error');
            }
        },
        
        async deleteUser(id) {
            if (!confirm('Are you sure you want to delete this user?')) return;
            
            try {
                await window.utils.api.delete(`/api/settings/users/${id}`);
                window.utils.showNotification('User deleted successfully', 'success');
                this.loadUsers();
            } catch (error) {
                console.error('Failed to delete user:', error);
                window.utils.showNotification('Failed to delete user', 'error');
            }
        },
        
        async changePassword() {
            if (!this.passwordChange.currentPassword || !this.passwordChange.newPassword) {
                window.utils.showNotification('All fields required', 'error');
                return;
            }
            
            try {
                await window.utils.api.put('/api/settings/password', {
                    current_password: this.passwordChange.currentPassword,
                    new_password: this.passwordChange.newPassword
                });
                window.utils.showNotification('Password updated successfully', 'success');
                this.passwordChange = {currentPassword: '', newPassword: ''};
            } catch (error) {
                console.error('Failed to change password:', error);
                window.utils.showNotification('Failed to update password', 'error');
            }
        }
    });
});
