import React, { useState, useEffect } from 'react';
import './App.css';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faMoon, faSun, faCloud, faComments, faCog, faStream, faFilter, faTimes, faSearch, faPause, faPlay, faTrash, faDownload, faPaperPlane, faRobot, faServer, faExclamationTriangle, faCheckCircle, faInfoCircle, faExclamationCircle, faSpinner, faPaperPlane as faSend, faPlus, faMinus } from '@fortawesome/free-solid-svg-icons';

function App() {
  const [darkMode, setDarkMode] = useState(false);
  const [logs, setLogs] = useState<any[]>([]);
  const [logTypeFilter, setLogTypeFilter] = useState('all');
  const [logLevelFilter, setLogLevelFilter] = useState('all');
  const [logSearch, setLogSearch] = useState('');
  const [logPauseState, setLogPauseState] = useState(false);
  const [connectionStatus, setConnectionStatus] = useState('checking');
  
  // Dark mode toggle
  const toggleDarkMode = () => {
    setDarkMode(!darkMode);
    document.documentElement.setAttribute('data-color-scheme', darkMode ? 'light' : 'dark');
  };

  // Enhanced log filtering with SSE
  useEffect(() => {
    let eventSource: EventSource;

    const connectEnhancedLogs = () => {
      const url = `/api/logs/enhanced?type=${logTypeFilter}&level=${logLevelFilter}&search=${encodeURIComponent(logSearch)}`;
      console.log('Connecting to enhanced logs SSE:', url);

      eventSource = new EventSource(url);

      eventSource.onopen = () => {
        console.log('Enhanced logs SSE connection established');
      };

      eventSource.onmessage = (e) => {
        const log = JSON.parse(e.data);
        setLogs(prevLogs => [log, ...prevLogs]);
      };

      eventSource.onerror = (error) => {
        console.error('Enhanced logs EventSource error:', error);
        setTimeout(connectEnhancedLogs, 2000);
      };
    };

    connectEnhancedLogs();

    return () => {
      if (eventSource) {
        eventSource.close();
      }
    };
  }, [logTypeFilter, logLevelFilter, logSearch]);

  // Connection status check
  useEffect(() => {
    const checkConnection = async () => {
      try {
        const response = await fetch('/api/health');
        const data = await response.json();
        if (data.status === 'healthy') {
          setConnectionStatus('healthy');
        } else {
          setConnectionStatus('unhealthy');
        }
      } catch (error) {
        setConnectionStatus('error');
      }
    };
    
    checkConnection();
    const interval = setInterval(checkConnection, 30000);
    return () => clearInterval(interval);
  }, []);

  // Log filtering functions
  const shouldDisplayLog = (log: any) => {
    // Filter out health check logs
    if (log.message && log.message.includes('Health check requested')) {
      return false;
    }
    
    if (log.endpoint === '/health') {
      return false;
    }
    
    // Apply type filter
    if (logTypeFilter !== 'all' && !log.type.startsWith(logTypeFilter)) {
      return false;
    }
    
    // Apply level filter
    if (logLevelFilter !== 'all' && log.level !== logLevelFilter) {
      return false;
    }
    
    // Apply search filter
    if (logSearch) {
      const searchLower = logSearch.toLowerCase();
      const messageMatch = log.message.toLowerCase().includes(searchLower);
      const contextMatch = log.context ? JSON.stringify(log.context).toLowerCase().includes(searchLower) : false;
      
      if (!messageMatch && !contextMatch) {
        return false;
      }
    }
    
    return true;
  };

  const JsonViewer = ({ data }: { data: any }) => {
    const [isExpanded, setIsExpanded] = useState(false);

    if (typeof data !== 'object' || data === null) {
      return <span>{JSON.stringify(data)}</span>;
    }

    const entries = Object.entries(data);

    return (
      <div>
        <span onClick={() => setIsExpanded(!isExpanded)} style={{ cursor: 'pointer' }}>
          {isExpanded ? '▼' : '▶'} {Array.isArray(data) ? `Array(${entries.length})` : `Object`}
        </span>
        {isExpanded && (
          <div style={{ marginLeft: '20px' }}>
            {entries.map(([key, value]) => (
              <div key={key}>
                <strong>{key}:</strong> <JsonViewer data={value} />
              </div>
            ))}
          </div>
        )}
      </div>
    );
  };

  // Log display component
  const LogEntry = ({ log }: { log: any }) => {
    const getLogTypeInfo = (type: string) => {
      const types: Record<string, { class: string; text: string; icon: any }> = {
        'mistral_request': { class: 'mistral-request', text: 'MISTRAL REQUEST', icon: faRobot },
        'mistral_response': { class: 'mistral-response', text: 'MISTRAL RESPONSE', icon: faRobot },
        'avi_request': { class: 'avi-request', text: 'AVI REQUEST', icon: faServer },
        'avi_response': { class: 'avi-response', text: 'AVI RESPONSE', icon: faServer },
        'error': { class: 'error-log', text: 'ERROR', icon: faExclamationTriangle },
        'success': { class: 'success-log', text: 'SUCCESS', icon: faCheckCircle },
        'warning': { class: 'warning-log', text: 'WARNING', icon: faExclamationCircle },
        'system': { class: 'system-log', text: 'SYSTEM', icon: faInfoCircle }
      };
      
      return types[type] || { class: 'system-log', text: 'SYSTEM', icon: faInfoCircle };
    };
    
    const logTypeInfo = getLogTypeInfo(log.type);
    
    return (
      <div className={`log-entry ${logTypeInfo.class}`}>
        <div className="log-header">
          <span className="log-type-badge badge bg-secondary">
            <FontAwesomeIcon icon={logTypeInfo.icon} /> {logTypeInfo.text}
          </span>
          <span className="log-timestamp small text-muted">
            {log.timestamp || new Date().toISOString()}
          </span>
        </div>
        <div className="log-content">
          <div className="log-message">{log.message}</div>
          {log.context && (
            <div className="log-context mt-2">
              <strong>Details:</strong>
              <JsonViewer data={log.context} />
            </div>
          )}
        </div>
      </div>
    );
  };

  return (
    <div className={`app-container ${darkMode ? 'dark-mode' : 'light-mode'}`}>
      <div className="app-header">
        <div className="header-content">
          <h3>
            <FontAwesomeIcon icon={faCloud} className="me-2" />
            VMware Avi LLM Agent
          </h3>
          <div className="header-controls">
            <div className="connection-status">
              <div className={`status-dot ${connectionStatus}`}></div>
              <small>{connectionStatus === 'healthy' ? 'Connected' : 'Connection Issues'}</small>
            </div>
            <button onClick={toggleDarkMode} className="btn btn-sm btn-outline-secondary">
              <FontAwesomeIcon icon={darkMode ? faSun : faMoon} />
            </button>
          </div>
        </div>
      </div>
      
      <div className="app-main">
        <div className="chat-column">
          <div className="chat-header">
            <h5><FontAwesomeIcon icon={faComments} className="me-2" /> Chat</h5>
          </div>
          <div className="chat-messages">
            <div className="welcome-message">
              Welcome to VMware Avi LLM Agent! Ask me anything about your Avi infrastructure.
            </div>
          </div>
          <div className="chat-input">
            <form>
              <div className="input-group">
                <input 
                  type="text" 
                  className="form-control"
                  placeholder="Type your message..."
                  autoFocus
                />
                <button type="submit" className="btn btn-primary">
                  <FontAwesomeIcon icon={faSend} />
                </button>
              </div>
            </form>
          </div>
        </div>
        
        <div className="logs-column">
          <div className="logs-header">
            <h5><FontAwesomeIcon icon={faStream} className="me-2" /> API Logs</h5>
            <div className="logs-controls">
              <div className="filter-controls">
                <select 
                  value={logTypeFilter} 
                  onChange={(e) => setLogTypeFilter(e.target.value)}
                  className="form-select form-select-sm"
                >
                  <option value="all">All Types</option>
                  <option value="mistral">Mistral</option>
                  <option value="avi">Avi</option>
                  <option value="user">User</option>
                  <option value="system">System</option>
                </select>
                
                <select 
                  value={logLevelFilter} 
                  onChange={(e) => setLogLevelFilter(e.target.value)}
                  className="form-select form-select-sm"
                >
                  <option value="all">All Levels</option>
                  <option value="info">Info</option>
                  <option value="warn">Warning</option>
                  <option value="error">Error</option>
                  <option value="debug">Debug</option>
                </select>
                
                <div className="input-group input-group-sm">
                  <input 
                    type="text" 
                    value={logSearch} 
                    onChange={(e) => setLogSearch(e.target.value)}
                    className="form-control" 
                    placeholder="Search..."
                  />
                  <button 
                    type="button" 
                    onClick={() => setLogSearch('')}
                    className="btn btn-outline-secondary"
                  >
                    <FontAwesomeIcon icon={faTimes} />
                  </button>
                </div>
                
                <button 
                  onClick={() => {
                    setLogTypeFilter('all');
                    setLogLevelFilter('all');
                    setLogSearch('');
                  }}
                  className="btn btn-sm btn-outline-secondary"
                >
                  <FontAwesomeIcon icon={faFilter} /> Clear
                </button>
              </div>
              
              <div className="logs-actions">
                <button 
                  onClick={() => setLogPauseState(!logPauseState)} 
                  className="btn btn-sm btn-outline-secondary"
                >
                  <FontAwesomeIcon icon={logPauseState ? faPlay : faPause} />
                  {logPauseState ? 'Resume' : 'Pause'}
                </button>
                <button 
                  onClick={() => setLogs([])} 
                  className="btn btn-sm btn-outline-danger"
                >
                  <FontAwesomeIcon icon={faTrash} /> Clear
                </button>
              </div>
            </div>
          </div>
          
          <div className="logs-display">
            {logs.filter(shouldDisplayLog).map((log, index) => (
              <LogEntry key={index} log={log} />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

export default App;