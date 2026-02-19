// Partners Store - Partner Management and Discovery

document.addEventListener('alpine:init', () => {
    Alpine.store('partners', {
        list: [],
        discoveryUrl: '',
        discoveryStatus: {
            show: false,
            type: 'info',
            message: ''
        },
        scannerActive: false,
        scanner: null,
        
        async loadPartners() {
            try {
                const data = await window.utils.api.get('/api/dashboard/partners');
                this.list = data.partners || [];
            } catch (error) {
                console.error('Failed to load partners:', error);
                this.list = [];
            }
        },
        
        async discoverPartner() {
            if (!this.discoveryUrl.trim()) {
                this.showDiscoveryStatus('error', 'Please enter a discovery URL');
                return;
            }
            
            this.showDiscoveryStatus('info', '⏳ Discovering partner...');
            
            try {
                const data = await window.utils.api.post('/api/dashboard/partners/discover', {
                    discovery_url: this.discoveryUrl
                });
                
                this.showDiscoveryStatus('success', `✅ Partner "${data.name}" discovered and added!`);
                this.discoveryUrl = '';
                this.loadPartners();
                Alpine.store('dashboard').loadMetrics();
            } catch (error) {
                this.showDiscoveryStatus('error', `❌ Failed to discover partner`);
            }
        },
        
        showDiscoveryStatus(type, message) {
            this.discoveryStatus = {show: true, type, message};
            setTimeout(() => {
                this.discoveryStatus.show = false;
            }, 5000);
        },
        
        toggleScanner() {
            if (this.scannerActive) {
                if (this.scanner) {
                    this.scanner.clear();
                }
                this.scannerActive = false;
            } else {
                this.startScanner();
            }
        },
        
        startScanner() {
            this.scanner = new Html5QrcodeScanner("qr-reader", {fps: 10, qrbox: 250});
            this.scanner.render(
                (decodedText) => this.onScanSuccess(decodedText),
                (error) => {} // Ignore scan errors
            );
            this.scannerActive = true;
        },
        
        onScanSuccess(decodedText) {
            if (this.scanner) {
                this.scanner.clear();
            }
            this.scannerActive = false;
            this.discoveryUrl = decodedText;
            this.discoverPartner();
        },
        
        downloadQR() {
            const link = document.createElement('a');
            link.href = '/api/dashboard/qr?size=512';
            link.download = 'fidex-partner-qr.png';
            link.click();
        },
        
        async deletePartner(id) {
            if (!confirm('Are you sure you want to remove this partner?')) return;
            
            try {
                await window.utils.api.delete(`/api/settings/partners/${id}`);
                window.utils.showNotification('Partner removed successfully', 'success');
                this.loadPartners();
                Alpine.store('dashboard').loadMetrics();
            } catch (error) {
                console.error('Failed to delete partner:', error);
                window.utils.showNotification('Failed to remove partner', 'error');
            }
        }
    });
});
