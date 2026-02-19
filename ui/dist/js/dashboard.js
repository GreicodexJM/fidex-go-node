// Dashboard Store - Metrics and Messages Management

document.addEventListener('alpine:init', () => {
    Alpine.store('dashboard', {
        metrics: {
            delivered: 0,
            queued: 0,
            failed: 0,
            successRate: '0%',
            partners: 0
        },
        messages: [],
        
        async loadMetrics() {
            try {
                const data = await window.utils.api.get('/api/dashboard/metrics');
                this.metrics = {
                    delivered: data.messages_delivered_24h || 0,
                    queued: data.messages_queued || 0,
                    failed: data.messages_failed || 0,
                    successRate: ((data.success_rate || 0) * 100).toFixed(1) + '%',
                    partners: data.active_partners || 0
                };
            } catch (error) {
                console.error('Failed to load metrics:', error);
            }
        },
        
        async loadMessages() {
            try {
                const data = await window.utils.api.get('/api/dashboard/messages?limit=10');
                this.messages = data.messages || [];
            } catch (error) {
                console.error('Failed to load messages:', error);
                this.messages = [];
            }
        },
        
        getStatusColor(status) {
            const colors = {
                'DELIVERED': 'bg-green-100 text-green-700',
                'QUEUED': 'bg-blue-100 text-blue-700',
                'FAILED': 'bg-red-100 text-red-700',
                'PROCESSING': 'bg-yellow-100 text-yellow-700'
            };
            return colors[status] || 'bg-slate-100 text-slate-700';
        }
    });
});
