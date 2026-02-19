// Main Application Store - FideX Node Dashboard with AlpineJS

document.addEventListener('alpine:init', () => {
    // Main App Store
    Alpine.store('app', {
        activeSection: 'dashboard',
        wsStatus: 'disconnected',
        wsStatusText: 'Connecting...',
        ws: null,
        
        init() {
            this.connectWebSocket();
            // Refresh metrics periodically
            setInterval(() => {
                if (this.activeSection === 'dashboard') {
                    Alpine.store('dashboard').loadMetrics();
                    Alpine.store('dashboard').loadMessages();
                }
            }, 30000);
        },
        
        setSection(section) {
            this.activeSection = section;
            
            // Load section-specific data
            if (section === 'dashboard') {
                Alpine.store('dashboard').loadMetrics();
                Alpine.store('dashboard').loadMessages();
                Alpine.store('partners').loadPartners();
            } else if (section === 'partners') {
                Alpine.store('partners').loadPartners();
            } else if (section === 'settings') {
                Alpine.store('settings').loadSettings();
            }
        },
        
        connectWebSocket() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            this.ws = new WebSocket(`${protocol}//${window.location.host}/api/dashboard/ws`);
            
            this.ws.onopen = () => {
                this.wsStatus = 'connected';
                this.wsStatusText = 'Connected';
            };
            
            this.ws.onmessage = (event) => {
                const msg = JSON.parse(event.data);
                this.handleWebSocketMessage(msg);
            };
            
            this.ws.onerror = () => {
                this.wsStatus = 'error';
                this.wsStatusText = 'Connection Error';
            };
            
            this.ws.onclose = () => {
                this.wsStatus = 'disconnected';
                this.wsStatusText = 'Disconnected';
                setTimeout(() => this.connectWebSocket(), 5000);
            };
        },
        
        handleWebSocketMessage(msg) {
            console.log('WebSocket message:', msg);
            
            if (msg.type === 'partner_added') {
                Alpine.store('partners').loadPartners();
                Alpine.store('dashboard').loadMetrics();
                window.utils.showNotification('New partner connected!', 'success');
            } else if (msg.type === 'message_new' || msg.type === 'message_status') {
                Alpine.store('dashboard').loadMessages();
                Alpine.store('dashboard').loadMetrics();
            }
        },
        
        async logout() {
            try {
                await fetch('/api/auth/logout', {method: 'POST'});
            } catch (error) {
                console.error('Logout failed:', error);
            }
            window.location.href = '/login';
        }
    });
});

// Initialize application when DOM is ready
document.addEventListener('DOMContentLoaded', async () => {
    // Load all HTML components
    await window.utils.initializeComponents();
    
    // Initialize Alpine stores
    Alpine.store('app').init();
    Alpine.store('dashboard').loadMetrics();
    Alpine.store('dashboard').loadMessages();
    Alpine.store('partners').loadPartners();
});
