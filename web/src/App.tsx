import { useState, useEffect, useCallback, useMemo } from 'react';
import { useI18n } from './i18n';
import type {
  CatalogItem,
  InstalledServer,
  ProxyHealthSummary,
  ReloadResult,
} from './types';
import { api } from './lib/api';
import { Header } from './components/Header';
import { StatusBanner } from './components/StatusBanner';
import { SearchFilterBar } from './components/SearchFilterBar';
import { MarketplaceGrid } from './components/MarketplaceGrid';
import { InstalledServersTable } from './components/InstalledServersTable';
import { InstallModal } from './components/InstallModal';
import { ServerDetailModal } from './components/ServerDetailModal';
import { ConfigDrawer } from './components/ConfigDrawer';
import { LogsViewerModal } from './components/LogsViewerModal';
import { ConfirmModal } from './components/ConfirmModal';
import { Server } from 'lucide-react';

export function App() {
  const { t } = useI18n();

  // Navigation tab
  const [activeTab, setActiveTab] = useState<'marketplace' | 'installed'>('marketplace');

  // Main Data States
  const [catalog, setCatalog] = useState<CatalogItem[]>([]);
  const [installedServers, setInstalledServers] = useState<InstalledServer[]>([]);
  const [health, setHealth] = useState<ProxyHealthSummary | null>(null);

  // Filters
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [selectedTransport, setSelectedTransport] = useState('all');

  // Loading States
  const [isLoadingCatalog, setIsLoadingCatalog] = useState(true);
  const [isLoadingInstalled, setIsLoadingInstalled] = useState(true);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isReloading, setIsReloading] = useState(false);
  const [lastReloadResult, setLastReloadResult] = useState<ReloadResult | null>(null);

  // Modal States
  const [installModalOpen, setInstallModalOpen] = useState(false);
  const [selectedCatalogItem, setSelectedCatalogItem] = useState<CatalogItem | null>(null);

  const [detailModalOpen, setDetailModalOpen] = useState(false);
  const [detailCatalogItem, setDetailCatalogItem] = useState<CatalogItem | null>(null);

  const [configDrawerOpen, setConfigDrawerOpen] = useState(false);
  const [selectedServerToEdit, setSelectedServerToEdit] = useState<InstalledServer | null>(null);

  const [logsModalOpen, setLogsModalOpen] = useState(false);
  const [logsServer, setLogsServer] = useState<InstalledServer | null>(null);

  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);
  const [serverToDelete, setServerToDelete] = useState<InstalledServer | null>(null);

  // Initial Fetch & Refresh
  const fetchData = useCallback(async (isInitial = false) => {
    if (!isInitial) setIsRefreshing(true);
    try {
      const [catalogData, installedData, healthData] = await Promise.all([
        api.getCatalog().catch(() => []),
        api.getInstalledServers().catch(() => []),
        api.getProxyHealth().catch(() => ({
          status: 'ready' as const,
          uptime: '1m',
          activeConnections: 0,
          totalServers: 0,
          activeServers: 0,
          serverCount: 0,
          lastReload: new Date().toISOString(),
        })),
      ]);

      setCatalog(catalogData);
      setInstalledServers(installedData);
      setHealth(healthData);
    } finally {
      setIsLoadingCatalog(false);
      setIsLoadingInstalled(false);
      setIsRefreshing(false);
    }
  }, []);

  useEffect(() => {
    fetchData(true);
    // Polling every 12 seconds
    const interval = setInterval(() => {
      api.getProxyHealth().then(setHealth).catch(() => {});
      api.getInstalledServers().then(setInstalledServers).catch(() => {});
    }, 12000);
    return () => clearInterval(interval);
  }, [fetchData]);

  // Actions
  const handleReloadProxy = async () => {
    setIsReloading(true);
    try {
      const res = await api.reloadProxy();
      setLastReloadResult(res);
      await fetchData();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Reload failed';
      setLastReloadResult({
        success: false,
        message: msg,
      });
    } finally {
      setIsReloading(false);
    }
  };

  const handleInstallServer = async (data: {
    catalogId?: string;
    name: string;
    transportType?: string;
    command?: string;
    args?: string[];
    url?: string;
    env?: Record<string, string>;
  }) => {
    const res = await api.installServer(data);
    if (res.reload) setLastReloadResult(res.reload);
    await fetchData();
    setActiveTab('installed');
  };

  const handleUpdateServer = async (
    id: string,
    data: {
      name?: string;
      enabled?: boolean;
      env?: Record<string, string>;
      args?: string[];
      command?: string;
      url?: string;
    }
  ) => {
    const res = await api.updateServer(id, data);
    if (res.reload) setLastReloadResult(res.reload);
    await fetchData();
  };

  const handleToggleEnabled = async (server: InstalledServer) => {
    const nextState = !server.enabled;
    await handleUpdateServer(server.id, { enabled: nextState });
  };

  const handleDeleteServer = async () => {
    if (!serverToDelete) return;
    const res = await api.uninstallServer(serverToDelete.id);
    if (res.reload) setLastReloadResult(res.reload);
    await fetchData();
  };

  // Filter Catalog
  const filteredCatalog = useMemo(() => {
    return catalog.filter((item) => {
      // Category filter
      if (selectedCategory !== 'all' && item.category !== selectedCategory) {
        return false;
      }
      // Transport filter
      if (selectedTransport !== 'all' && item.transportType !== selectedTransport) {
        return false;
      }
      // Search query
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const matchName = item.name.toLowerCase().includes(q);
        const matchDesc = item.description.toLowerCase().includes(q);
        const matchDescFr = item.descriptionFr ? item.descriptionFr.toLowerCase().includes(q) : false;
        const matchAuthor = item.author ? item.author.toLowerCase().includes(q) : false;
        const matchTags = item.tags?.some((t) => t.toLowerCase().includes(q));
        if (!matchName && !matchDesc && !matchDescFr && !matchAuthor && !matchTags) {
          return false;
        }
      }
      return true;
    });
  }, [catalog, selectedCategory, selectedTransport, searchQuery]);

  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex flex-col font-sans selection:bg-cyan-500/30 selection:text-cyan-200">
      {/* Top Header */}
      <Header
        activeTab={activeTab}
        onTabChange={setActiveTab}
        installedCount={installedServers.length}
        onRefresh={() => fetchData()}
        isRefreshing={isRefreshing}
        onAddCustom={() => {
          setSelectedCatalogItem(null);
          setInstallModalOpen(true);
        }}
      />

      {/* Main Container */}
      <main className="flex-1 max-w-7xl w-full mx-auto px-4 sm:px-6 lg:px-8 py-6 space-y-6">
        {/* Status & Health Banner */}
        <StatusBanner
          health={health}
          onReload={handleReloadProxy}
          isReloading={isReloading}
          lastReloadResult={lastReloadResult}
        />

        {/* Tab View: Marketplace */}
        {activeTab === 'marketplace' && (
          <div className="space-y-6 animate-fade-in">
            {/* Search and Category Filters */}
            <SearchFilterBar
              searchQuery={searchQuery}
              onSearchChange={setSearchQuery}
              selectedCategory={selectedCategory}
              onCategoryChange={setSelectedCategory}
              selectedTransport={selectedTransport}
              onTransportChange={setSelectedTransport}
            />

            {/* Marketplace Grid */}
            <MarketplaceGrid
              items={filteredCatalog}
              installedServers={installedServers}
              isLoading={isLoadingCatalog}
              onInstall={(item) => {
                setSelectedCatalogItem(item);
                setInstallModalOpen(true);
              }}
              onViewDetails={(item) => {
                setDetailCatalogItem(item);
                setDetailModalOpen(true);
              }}
              onManage={(server) => {
                setSelectedServerToEdit(server);
                setConfigDrawerOpen(true);
              }}
            />
          </div>
        )}

        {/* Tab View: Installed Servers */}
        {activeTab === 'installed' && (
          <div className="space-y-6 animate-fade-in">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-sm font-bold text-slate-100 flex items-center gap-2">
                  <Server className="w-4 h-4 text-cyan-400" />
                  <span>{t('navigation.installedServers')}</span>
                </h2>
                <p className="text-xs text-slate-400">
                  Manage active connections, environment variables, and proxy routes.
                </p>
              </div>
            </div>

            <InstalledServersTable
              servers={installedServers}
              onToggleEnabled={handleToggleEnabled}
              onEdit={(server) => {
                setSelectedServerToEdit(server);
                setConfigDrawerOpen(true);
              }}
              onViewLogs={(server) => {
                setLogsServer(server);
                setLogsModalOpen(true);
              }}
              onDelete={(server) => {
                setServerToDelete(server);
                setConfirmDeleteOpen(true);
              }}
              onBrowseMarketplace={() => setActiveTab('marketplace')}
              isLoading={isLoadingInstalled}
            />
          </div>
        )}
      </main>

      {/* Modals & Slide-overs */}
      <InstallModal
        isOpen={installModalOpen}
        onClose={() => {
          setInstallModalOpen(false);
          setSelectedCatalogItem(null);
        }}
        catalogItem={selectedCatalogItem}
        onInstall={handleInstallServer}
      />

      <ServerDetailModal
        isOpen={detailModalOpen}
        onClose={() => {
          setDetailModalOpen(false);
          setDetailCatalogItem(null);
        }}
        item={detailCatalogItem}
        onInstall={(item) => {
          setSelectedCatalogItem(item);
          setInstallModalOpen(true);
        }}
      />

      <ConfigDrawer
        isOpen={configDrawerOpen}
        onClose={() => {
          setConfigDrawerOpen(false);
          setSelectedServerToEdit(null);
        }}
        server={selectedServerToEdit}
        onSave={handleUpdateServer}
      />

      <LogsViewerModal
        isOpen={logsModalOpen}
        onClose={() => {
          setLogsModalOpen(false);
          setLogsServer(null);
        }}
        server={logsServer}
      />

      <ConfirmModal
        isOpen={confirmDeleteOpen}
        onClose={() => {
          setConfirmDeleteOpen(false);
          setServerToDelete(null);
        }}
        onConfirm={handleDeleteServer}
        title={t('confirmModal.deleteTitle')}
        message={t('confirmModal.deleteMessage', { name: serverToDelete?.name || '' })}
        confirmLabel={t('common.delete')}
        isDestructive={true}
      />

      {/* Footer */}
      <footer className="border-t border-slate-800/80 bg-slate-950 py-5 text-center text-xs text-slate-400">
        <div className="max-w-7xl mx-auto px-4 flex flex-col sm:flex-row items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <span>Powered by</span>
            <a
              href="https://github.com/tbxark/mcp-proxy"
              target="_blank"
              rel="noopener noreferrer"
              className="text-cyan-400 font-semibold hover:underline"
            >
              mcp-proxy
            </a>
            <span>&bull; Model Context Protocol</span>
          </div>
          <div className="flex items-center gap-1 text-[11px] text-slate-400">
            <span>Embedded Single-Binary Architecture</span>
          </div>
        </div>
      </footer>
    </div>
  );
}
