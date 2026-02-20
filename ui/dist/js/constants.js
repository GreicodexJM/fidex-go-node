// API Route Constants for FideX Node
// This module fetches route constants from the backend to ensure consistency

class RouteConstants {
    constructor() {
        this.routes = null;
        this.initialized = false;
        this.initPromise = null;
    }

    async init() {
        if (this.initialized) {
            return this.routes;
        }

        if (this.initPromise) {
            return this.initPromise;
        }

        this.initPromise = this._fetchConstants();
        return this.initPromise;
    }

    async _fetchConstants() {
        try {
            const response = await fetch('/api/constants');
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            this.routes = await response.json();
            this.initialized = true;
            return this.routes;
        } catch (error) {
            console.error('Failed to load route constants:', error);
            // Fallback to hardcoded values if fetch fails
            this.routes = this._getFallbackConstants();
            this.initialized = true;
            return this.routes;
        }
    }

    _getFallbackConstants() {
        return {
            // Authentication routes
            authLogin: '/api/auth/login',
            authLogout: '/api/auth/logout',
            authMe: '/api/auth/me',

            // Dashboard routes
            dashboardWS: '/api/dashboard/ws',
            dashboardQR: '/api/dashboard/qr',
            dashboardMetrics: '/api/dashboard/metrics',
            dashboardMessages: '/api/dashboard/messages',
            dashboardPartners: '/api/dashboard/partners',
            discoverPartner: '/api/dashboard/partners/discover',

            // Settings routes
            settingsConfig: '/api/settings/config',
            settingsRotateKeys: '/api/settings/rotate-keys',
            settingsUsers: '/api/settings/users',
            settingsUserById: '/api/settings/users/{id}',
            settingsPassword: '/api/settings/password',
            settingsPartners: '/api/settings/partners',
            settingsPartnerById: '/api/settings/partners/{id}',

            // Page routes
            login: '/login',
            dashboard: '/dashboard'
        };
    }

    // Helper method to replace path parameters (e.g., {id})
    buildPath(template, params = {}) {
        let path = template;
        for (const [key, value] of Object.entries(params)) {
            path = path.replace(`{${key}}`, value);
        }
        return path;
    }

    // Getters for easy access
    get auth() {
        return {
            login: this.routes.authLogin,
            logout: this.routes.authLogout,
            me: this.routes.authMe
        };
    }

    get dashboard() {
        return {
            ws: this.routes.dashboardWS,
            qr: this.routes.dashboardQR,
            metrics: this.routes.dashboardMetrics,
            messages: this.routes.dashboardMessages,
            partners: this.routes.dashboardPartners,
            discover: this.routes.discoverPartner
        };
    }

    get settings() {
        return {
            config: this.routes.settingsConfig,
            rotateKeys: this.routes.settingsRotateKeys,
            users: this.routes.settingsUsers,
            userById: (id) => this.buildPath(this.routes.settingsUserById, { id }),
            password: this.routes.settingsPassword,
            partners: this.routes.settingsPartners,
            partnerById: (id) => this.buildPath(this.routes.settingsPartnerById, { id })
        };
    }

    get pages() {
        return {
            login: this.routes.login,
            dashboard: this.routes.dashboard
        };
    }
}

// Create and export singleton instance
const ROUTES = new RouteConstants();

// Export for use in other modules
window.ROUTES = ROUTES;
