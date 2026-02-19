// Utility Functions for FideX Node Dashboard

// API Helper
const api = {
    async get(url) {
        const response = await fetch(url);
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
    },
    
    async post(url, data) {
        const response = await fetch(url, {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(data)
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
    },
    
    async put(url, data) {
        const response = await fetch(url, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(data)
        });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
    },
    
    async delete(url) {
        const response = await fetch(url, {method: 'DELETE'});
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
    }
};

// Show notification
function showNotification(message, type = 'info') {
    const div = document.createElement('div');
    const bgColor = type === 'success' ? 'bg-green-600' : 
                    type === 'error' ? 'bg-red-600' : 'bg-blue-600';
    
    div.className = `notification-enter fixed top-4 right-4 px-6 py-3 rounded-lg shadow-lg text-white z-50 ${bgColor}`;
    div.textContent = message;
    document.body.appendChild(div);
    
    setTimeout(() => {
        div.classList.remove('notification-enter');
        div.classList.add('notification-exit');
        setTimeout(() => div.remove(), 300);
    }, 3000);
}

// Load HTML component
async function loadComponent(url) {
    try {
        const response = await fetch(url);
        if (!response.ok) throw new Error(`Failed to load component: ${url}`);
        return await response.text();
    } catch (error) {
        console.error('Error loading component:', error);
        return `<div class="text-red-600">Error loading component</div>`;
    }
}

// Initialize all components
async function initializeComponents() {
    const components = [
        {selector: '#sidebar-container', url: '/components/sidebar.html'},
        {selector: '#dashboard-container', url: '/components/dashboard-section.html'},
        {selector: '#partners-container', url: '/components/partners-section.html'},
        {selector: '#settings-container', url: '/components/settings-section.html'},
        {selector: '#ws-status-container', url: '/components/ws-status.html'}
    ];
    
    for (const component of components) {
        const element = document.querySelector(component.selector);
        if (element) {
            const html = await loadComponent(component.url);
            element.innerHTML = html;
        }
    }
}

// Export for use in other modules
window.utils = {api, showNotification, loadComponent, initializeComponents};
