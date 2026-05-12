
        let columnsData = [];
        let currentSearchTerm = "";
        let archivedTickets = [];
        let availableBoards = [];  // Store all boards from /api/boards
        let currentBoardId = "";   // Currently selected board ID
        
        // ============================================
        // SSE (Server-Sent Events) for real-time updates
        // ============================================
        let sseConnected = false;
        let eventSource = null;
        
        function initSSE() {
            if ('EventSource' in window) {
                eventSource = new EventSource('/events');
                
                eventSource.onopen = function() {
                    console.log('SSE: Connected to server');
                    sseConnected = true;
                    updateConnectionStatus();
                };
                
                eventSource.onerror = function(error) {
                    console.error('SSE: Error:', error);
                    sseConnected = false;
                    updateConnectionStatus();
                    // Attempt reconnection after 3 seconds
                    setTimeout(function() {
                        if (eventSource.readyState === EventSource.CLOSED) {
                            console.log('SSE: Reconnecting...');
                            initSSE();
                        }
                    }, 3000);
                };
                
                eventSource.addEventListener('board_update', function(e) {
                    const data = JSON.parse(e.data);
                    handleBoardUpdate(data);
                });
            } else {
                console.warn('EventSource not supported - real-time updates disabled');
            }
        }
        
        function updateConnectionStatus() {
            // Could add a visual indicator in the UI if desired
            const status = sseConnected ? 'connected' : 'disconnected';
            console.log('SSE connection:', status);
        }
        
        function handleBoardUpdate(event) {
            console.log('Received SSE event:', event.type, event.ticket_id);
            
            // Handle different event types by refreshing relevant views
            switch (event.type) {
                case 'create':
                case 'move':
                case 'update':
                case 'delete':
                case 'release':
                    // Refresh the main board view
                    if (!document.getElementById('boardView').classList.contains('hidden')) {
                        loadBoard().catch(e => console.error('Failed to refresh board:', e));
                    }
                    break;
                    
                case 'archive':
                case 'unarchive':
                    // Refresh both views as tickets moved between them
                    if (!document.getElementById('boardView').classList.contains('hidden')) {
                        loadBoard().catch(e => console.error('Failed to refresh board:', e));
                    }
                    if (!document.getElementById('archiveView').classList.contains('hidden')) {
                        loadArchive().catch(e => console.error('Failed to refresh archive:', e));
                    }
                    break;
                    
                default:
                    console.log('Unknown event type:', event.type);
            }
        }
        
        // Switch between board view and archive view
        function switchView(view) {
            const boardView = document.getElementById('boardView');
            const archiveView = document.getElementById('archiveView');
            const tabBoard = document.getElementById('tab-board');
            const tabArchive = document.getElementById('tab-archive');
            
            if (view === 'archive') {
                boardView.classList.add('hidden');
                archiveView.classList.remove('hidden');
                tabBoard.className = "px-4 py-2 text-sm font-medium rounded-lg transition-all hover:bg-gray-700 text-gray-300";
                tabArchive.className = "px-4 py-2 text-sm font-medium rounded-lg transition-all bg-blue-600 text-white";
                loadArchive();
            } else {
                archiveView.classList.add('hidden');
                boardView.classList.remove('hidden');
                tabArchive.className = "px-4 py-2 text-sm font-medium rounded-lg transition-all hover:bg-gray-700 text-gray-300";
                tabBoard.className = "px-4 py-2 text-sm font-medium rounded-lg transition-all bg-blue-600 text-white";
            }
        }
        
        // Load archived tickets from API
        async function loadArchive() {
            try {
                const res = await fetch('/api/archived', { headers: getAuthHeaders() });
                if (!res.ok) {
                    console.error("Failed to load archive:", res.status, await res.text());
                    document.getElementById('archiveList').innerHTML = '<p class="text-gray-500 text-center py-8">Error loading archive</p>';
                    return;
                }
                archivedTickets = await res.json();
                renderArchive();
            } catch (e) {
                console.error("Failed to load archive:", e);
                document.getElementById('archiveList').innerHTML = '<p class="text-gray-500 text-center py-8">Error loading archive</p>';
            }
        }
        
        // Render archived tickets list
        function renderArchive() {
            const listEl = document.getElementById('archiveList');
            const countEl = document.getElementById('archiveCount');
            
            if (!archivedTickets || archivedTickets.length === 0) {
                listEl.innerHTML = '<p class="text-gray-500 text-center py-8">No archived tickets</p>';
                countEl.textContent = '0 tickets';
                return;
            }
            
            // Filter by search term if exists
            const archiveSearchInput = document.getElementById('archiveSearchInput');
            let filteredTickets = archivedTickets;
            if (archiveSearchTerm) {
                filteredTickets = archivedTickets.filter(t => 
                    (t.title && t.title.toLowerCase().includes(archiveSearchTerm)) ||
                    (t.description && t.description.toLowerCase().includes(archiveSearchTerm)) ||
                    (t.board_id && t.board_id.toLowerCase().includes(archiveSearchTerm))
                );
            }
            
            countEl.textContent = `${archivedTickets.length} tickets`;
            
            listEl.innerHTML = filteredTickets.map(ticket => `
                <div class="bg-gray-800 border border-gray-700 rounded-lg p-4 flex justify-between items-start">
                    <div class="flex-1 min-w-0">
                        <div class="flex items-center gap-3 mb-2">
                            <h3 class="text-white font-medium truncate">${escapeHtml(ticket.title || 'Untitled')}</h3>
                            <span class="text-xs text-gray-500 px-2 py-1 bg-gray-700 rounded">${escapeHtml(ticket.board_id || 'unknown')}</span>
                        </div>
                        ${ticket.description ? `<p class="text-sm text-gray-400 line-clamp-2">${escapeHtml(ticket.description.substring(0, 200))}${ticket.description.length > 200 ? '...' : ''}</p>` : ''}
                        <div class="mt-2 flex items-center gap-4 text-xs text-gray-500">
                            ${ticket.archived_at ? `<span>🗄️ Archived: ${formatDate(ticket.archived_at)}</span>` : ''}
                            ${ticket.priority ? `<span><span class="${getPriorityColor(ticket.priority)}">${escapeHtml(ticket.priority)}</span></span>` : ''}
                        </div>
                    </div>
                    <button onclick="unarchiveTicket('${ticket.id}')" 
                            class="bg-green-600 hover:bg-green-500 text-white px-4 py-2 rounded-lg text-sm font-medium transition-colors ml-4"
                            title="Restore this ticket">
                        Restore
                    </button>
                </div>
            `).join('');
        }
        
        // Archive search filter
        let archiveSearchTerm = "";
        function filterArchive() {
            const input = document.getElementById('archiveSearchInput');
            archiveSearchTerm = input.value.toLowerCase().trim();
            renderArchive();
        }
        
// Unarchive a ticket (restore to board)
	async function unarchiveTicket(ticketId) {
		// Get the archived ticket data for board_id and column
		const ticket = archivedTickets.find(t => t.id === ticketId);
		if (!ticket) return;
		
		try {
			const res = await fetch(`/api/unarchive/${ticketId}`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json', ...getAuthHeaders() },
           body: JSON.stringify({
                        board_id: currentBoardId || ticket.board_id,  // Use currently selected board
                        column: "todo" // Default to To Do when restoring (ticket-8c6759849c1f59ef: audit for hardcoded value + past JS/layout regressions; stable per memory "working over fancy")
                    })
			});
			
			if (res.ok) {
				alert('Ticket "' + escapeHtml(ticket.title) + '" has been restored to "To Do"');
				await loadArchive(); // Refresh archive list (removes restored ticket)
				await loadBoard();  // Refresh board view (shows restored ticket immediately)
			} else {
				const err = await res.json();
				alert('Failed to restore ticket: ' + (err.error || 'Unknown error'));
			}
		} catch (e) {
			console.error("Unarchive failed:", e);
			alert('Error restoring ticket');
		}
	}

        // Utility: Escape HTML to prevent XSS
        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
        
        // Utility: Format date for display
        function formatDate(dateStr) {
            if (!dateStr) return '';
            const date = new Date(dateStr);
            return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'});
        }
        
        // Utility: Get priority color class
        function getPriorityColor(priority) {
            const colors = {
                'critical': 'text-orange-500 font-bold',
                'high': 'text-red-400',
                'medium': 'text-yellow-400',
                'low': 'text-green-400'
            };
            return colors[priority?.toLowerCase()] || 'text-gray-400';
        }
        
        async function loadBoard() {
            try {
                const res = await fetch('/api/boards');
                const boards = await res.json();
                
                // Store all available boards globally
                availableBoards = boards || [];
                
                if (availableBoards.length > 0) {
                    console.log("Loaded", availableBoards.length, "boards:", availableBoards.map(b => b.id).join(", "));
                    
                    // Populate board selector dropdown
                    populateBoardDropdown();
                    
                    // Check for saved board selection and restore it if valid
                    const savedBoardId = loadSavedBoard();
                    let initialBoardId = availableBoards[0].id;  // Default to first board
                    
                    if (savedBoardId && availableBoards.some(b => b.id === savedBoardId)) {
                        console.log("Restoring saved board selection:", savedBoardId);
                        initialBoardId = savedBoardId;
                    } else if (savedBoardId) {
                        console.log("Saved board no longer available, using first board");
                    }
                    
                    // Load the selected board data with full ticket details
                    currentBoardId = initialBoardId;
                    
                    // Fetch full board details from /api/boards/{id} to get complete ticket data including descriptions
                    try {
                        const boardRes = await fetch(`/api/boards/${initialBoardId}`);
                        if (boardRes.ok) {
                            const selectedBoard = await boardRes.json();
                            columnsData = (selectedBoard.columns || []).map(col => ({
                                ...col,
                                tickets: col.tickets || []  // Ensure tickets is always an array
                            }));
                            console.log("Loaded board:", selectedBoard.title || "Unknown", "with", columnsData.length, "columns");
                        } else {
                            // Fallback to availableBoards data if fetch fails
                            const fallbackBoard = availableBoards.find(b => b.id === initialBoardId) || availableBoards[0];
                            columnsData = (fallbackBoard.columns || []).map(col => ({
                                ...col,
                                tickets: col.tickets || []
                            }));
                            console.log("Using fallback board data:", fallbackBoard.title || "Unknown");
                        }
                    } catch (fetchErr) {
                        console.error("Failed to fetch full board details for", initialBoardId, ":", fetchErr);
                        // Fallback to availableBoards data
                        const fallbackBoard = availableBoards.find(b => b.id === initialBoardId) || availableBoards[0];
                        columnsData = (fallbackBoard.columns || []).map(col => ({
                            ...col,
                            tickets: col.tickets || []
                        }));
                    }
                } else {
                    console.error("No boards returned");
                }
            } catch (e) {
                console.error("Failed to load boards:", e);
            }
            renderBoard();
        }

        // Populate the board selector dropdown with available boards
        function populateBoardDropdown() {
            const select = document.getElementById('boardSelect');
            if (!select) return;
            
            // Clear existing options
            select.innerHTML = '';
            
            // Add options for each board
            availableBoards.forEach(board => {
                const option = document.createElement('option');
                option.value = board.id;
                option.textContent = board.title || board.id;
                select.appendChild(option);
            });
            
            // Set to saved selection or first board as default (ticket-f6b54abf851216b6)
            const savedBoardId = loadSavedBoard();
            if (savedBoardId && availableBoards.some(b => b.id === savedBoardId)) {
                select.value = savedBoardId;
            } else if (availableBoards.length > 0) {
                select.value = availableBoards[0].id;
            }
        }

        // Switch to a different board by ID - fetches and renders the selected board
        async function switchBoard(boardId) {
            try {
                console.log("Switching to board:", boardId);
                
                // Update current board ID and persist it
                currentBoardId = boardId;
                saveSelectedBoard(boardId);  // Save selection for next page load
                
                // Fetch the specific board data
                const res = await fetch(`/api/boards/${boardId}`);
                if (!res.ok) throw new Error(`HTTP ${res.status}: Failed to fetch board ${boardId}`);
                
                const boardData = await res.json();
                
                // Parse and populate columnsData from board.columns
                columnsData = (boardData.columns || []).map(col => ({
                    ...col,
                    tickets: col.tickets || []  // Ensure tickets is always an array
                }));
                
                console.log("Board switched:", boardData.title || boardId, "with", columnsData.length, "columns");
                
                // Update dropdown to reflect current selection
                const select = document.getElementById('boardSelect');
                if (select) {
                    select.value = boardId;
                }
                
                // Render the selected board
                renderBoard();
                
            } catch (e) {
                console.error("Failed to switch to board", boardId, ":", e);
                alert('Failed to load board: ' + e.message);
                
                // Fallback: try to reload first available board
                if (availableBoards.length > 0) {
                    await switchBoard(availableBoards[0].id);
                }
            }
        }
        
        function filterTickets() {
            const input = document.getElementById('searchInput');
            currentSearchTerm = input.value.toLowerCase().trim();
            renderBoard();
        }
        
        function renderBoard() {
            const board = document.getElementById('board');
            board.innerHTML = '';
            
            columnsData.forEach(column => {
                const colEl = document.createElement('div');
                // col-span-2 with grid-cols-6 ensures 3 equal-width columns (fixes vertical stacking regression)
                colEl.className = 'column p-4 flex flex-col col-span-2';
                colEl.dataset.columnId = column.id;
                
                // Filter tickets if search term exists
                let filteredTickets = column.tickets;
                if (currentSearchTerm) {
                    filteredTickets = column.tickets.filter(ticket => 
                        (ticket.title && ticket.title.toLowerCase().includes(currentSearchTerm)) ||
                        (ticket.description && ticket.description.toLowerCase().includes(currentSearchTerm)) ||
                        (ticket.assignee && ticket.assignee.toLowerCase().includes(currentSearchTerm))
                    );
                }
                
                colEl.innerHTML = `
                    <div class="column-header px-4 py-3 mb-4 rounded-t-lg">
                        <div class="flex items-center justify-between">
                            <div class="font-semibold text-lg">${escapeHtml(column.title)}</div>
                            <div class="bg-gray-700 text-xs px-2 py-0.5 rounded-full">${filteredTickets.length}</div>
                        </div>
                    </div>
                    <div class="tickets flex-1 space-y-3 min-h-[400px] p-1" 
                         ondrop="drop(event)" 
                         ondragover="allowDrop(event)"
                         ondragleave="dragLeave(event)">
                        ${filteredTickets.map(ticket => `
                            <div class="ticket p-5 rounded-2xl cursor-move shadow-md" 
                                 draggable="true"
                                 ondragstart="drag(event)"
                                 onclick="showTicketDetail('${ticket.id}')"
                                 data-ticket-id="${escapeHtml(ticket.id)}"
                                 data-column-id="${escapeHtml(column.id)}">
                                <div class="mb-2">
                                    <div class="font-medium text-white text-[15px] leading-tight">${escapeHtml(ticket.title)}</div>
                                    <div class="text-[9px] text-gray-500 font-mono mt-1">${escapeHtml(ticket.id)}</div>
                                </div>
                                <div class="flex flex-wrap gap-2">
                               <span class="inline-block px-3 py-1 text-[10px] font-medium rounded-xl ${ticket.priority === 'critical' ? 'bg-red-900 text-red-200' : ticket.priority === 'high' ? 'bg-orange-900 text-orange-200' : 'bg-blue-900 text-blue-300'}">
                                        ${escapeHtml(ticket.priority || 'medium')}
                                    </span>
                                    ${ticket.assignee ? `<span class="inline-block px-3 py-1 text-[10px] bg-gray-700 text-gray-300 rounded-xl">${escapeHtml(ticket.assignee)}</span>` : ''}
                                    ${ticket.due_date ? `<span class="inline-block px-3 py-1 text-[10px] bg-amber-900 text-amber-200 rounded-xl">📅 ${escapeHtml(ticket.due_date)}</span>` : ''}
                                </div>
                                ${ticket.labels && ticket.labels.length > 0 ? `
                                <div class="flex flex-wrap gap-1 mt-3">
                                    ${ticket.labels.map(label => `
                                        <span class="text-[9px] px-2 py-px bg-gray-800 text-gray-400 rounded">${escapeHtml(label)}</span>
                                    `).join('')}
                                </div>` : ''}
                                ${ticket.subtasks && ticket.subtasks.length > 0 ? (() => {
                                    const completed = ticket.subtasks.filter(st => st.completed).length;
                                    const total = ticket.subtasks.length;
                                    return `<div class="mt-3 flex items-center gap-2">
                                        <span class="text-[9px] text-gray-500">${completed}/${total}</span>
                                        <div class="flex-1 h-1 bg-gray-700 rounded-full overflow-hidden">
                                            <div class="h-full bg-blue-600 rounded-full" style="width:${(completed/total)*100}%"></div>
                                        </div>
                                    </div>`;
                                })() : ''}
                            </div>
                        `).join('')}
                    </div>
                `;
                
                board.appendChild(colEl);
            });
        }
        
        function allowDrop(ev) {
            ev.preventDefault();
            
            // Add visual feedback when dragging over a column
            const columnEl = ev.currentTarget.closest('.column');
            if (columnEl) {
                document.querySelectorAll('.column.drag-over').forEach(c => c.classList.remove('drag-over'));
                columnEl.classList.add('drag-over');
                
                // Show drop indicator at appropriate position
                showDropIndicator(ev, columnEl);
            }
        }

        function dragLeave(ev) {
            const columnEl = ev.currentTarget.closest('.column');
            if (columnEl) {
                columnEl.classList.remove('drag-over');
                hideAllDropIndicators();
            }
        }

        function showDropIndicator(ev, columnEl) {
            // Hide all existing indicators first
            hideAllDropIndicators();
            
            const tickets = Array.from(columnEl.querySelectorAll('.ticket'));
            if (tickets.length === 0) {
                // Empty column - show at top
                const indicator = document.createElement('div');
                indicator.className = 'drop-indicator visible';
                indicator.dataset.position = 'top';
                columnEl.insertBefore(indicator, columnEl.firstChild);
                return;
            }
            
            // Find closest ticket based on Y position
            let closestTicket = null;
            let closestDistance = Infinity;
            
            for (const ticket of tickets) {
                const rect = ticket.getBoundingClientRect();
                const distance = Math.abs(ev.clientY - rect.top - rect.height / 2);
                if (distance < closestDistance) {
                    closestDistance = distance;
                    closestTicket = ticket;
                }
            }
            
            if (!closestTicket) return;
            
            // Create indicator
            const indicator = document.createElement('div');
            indicator.className = 'drop-indicator visible';
            
            // Position above or below based on where in the ticket we're hovering
            const rect = closestTicket.getBoundingClientRect();
            const relativeY = ev.clientY - rect.top;
            
            if (relativeY < rect.height / 2) {
                // Above the ticket
                indicator.dataset.position = 'above';
                closestTicket.parentElement.insertBefore(indicator, closestTicket);
            } else {
                // Below the ticket
                indicator.dataset.position = 'below';
                const nextSibling = closestTicket.nextElementSibling;
                if (nextSibling) {
                    closestTicket.parentElement.insertBefore(indicator, nextSibling);
                } else {
                    closestTicket.parentElement.appendChild(indicator);
                }
            }
        }

        function hideAllDropIndicators() {
            document.querySelectorAll('.drop-indicator').forEach(el => el.remove());
        }

        function drag(ev) {
            const ticketEl = ev.target.closest('.ticket');
            if (!ticketEl) return;
            ev.dataTransfer.setData("ticketId", ticketEl.dataset.ticketId);
            ev.dataTransfer.setData("fromColumn", ticketEl.dataset.columnId);
            ticketEl.classList.add('dragging');
        }
        
 async function drop(ev) {
            ev.preventDefault();
            
            // Remove drag-over styling and hide indicators
            document.querySelectorAll('.column.drag-over').forEach(c => c.classList.remove('drag-over'));
            hideAllDropIndicators();
            
            // Remove dragging class from any previously dragged element
            const oldDragging = document.querySelector('.ticket.dragging');
            if (oldDragging) oldDragging.classList.remove('dragging');
            
            const ticketId = ev.dataTransfer.getData("ticketId");
            const fromColumn = ev.dataTransfer.getData("fromColumn");
            const columnEl = ev.currentTarget.closest('.column');
            const toColumn = columnEl.dataset.columnId;
            
            if (!ticketId || !toColumn) {
                console.error('Drop failed: missing ticketId or toColumn');
                return;
            }
            
            if (fromColumn === toColumn) return;
            
            // Update UI immediately and wait for API confirmation
            const result = await moveTicket(ticketId, toColumn, currentBoardId);
            
            if (!result.ok) {
                console.error('Move failed:', result.error);
                alert('Failed to move ticket: ' + (result.error || 'Unknown error'));
                return;
            }
            
            // Refresh board after successful move
            await loadBoard();
        }

        // Map column IDs to v1 status names (single source of truth for frontend->API mapping).
        function columnIdToStatus(columnId) {
            const map = {
                'backlog-0': 'BACKLOG',
                'todo-0': 'TODO',
                'inprogress-0': 'IN_PROGRESS',
                'review-0': 'REVIEW',
                'done-0': 'DONE',
                'cancelled-0': 'CANCELLED'
            };
            return map[columnId] || columnId;
        }

        async function moveTicket(ticketId, toColumn, boardId) {
            const targetStatus = columnIdToStatus(toColumn);
            
            try {
                const res = await fetch('/api/v1/tickets/' + ticketId + '/move', {
                    method: 'POST',
                    headers: { 
                        'Content-Type': 'application/json',
                        ...getAuthHeaders()
                    },
                    body: JSON.stringify({
                        target_status: targetStatus
                    })
                });
                
                if (!res.ok) {
                    const errorData = await res.json().catch(() => ({}));
                    return { ok: false, error: errorData.error || `HTTP ${res.status}` };
                }
                
                const data = await res.json();
                console.log('Ticket moved successfully:', ticketId, 'to', targetStatus);
                return { ok: true, data: data };
            } catch (err) {
                console.error('Move request failed:', err);
                return { ok: false, error: err.message };
            }
        }
        
        function showNewTicketModal() {
            document.getElementById('newTicketModal').classList.remove('hidden');
            document.getElementById('ticketTitle').focus();
        }
        
        function hideNewTicketModal() {
            document.getElementById('newTicketModal').classList.add('hidden');
            // Clear form
            document.getElementById('ticketTitle').value = '';
            document.getElementById('ticketDesc').value = '';
        }
        
        function updateTitleCount() {
            const input = document.getElementById('ticketTitle');
            const count = document.getElementById('titleCount');
            count.textContent = input.value.length + '/128';
            if (input.value.length > 110) {
                count.style.color = '#f59e0b';
            } else {
                count.style.color = '';
            }
        }
        
        function updateDescCount() {
            const input = document.getElementById('ticketDesc');
            const count = document.getElementById('descCount');
            count.textContent = input.value.length + '/16384';
            if (input.value.length > 14000) {
                count.style.color = '#f59e0b';
            } else {
                count.style.color = '';
            }
        }
        
        function updateEditDescCount() {
            const input = document.getElementById('detailDescriptionEdit');
            const count = document.getElementById('editDescCount');
            count.textContent = input.value.length + '/16384';
            if (input.value.length > 14000) {
                count.style.color = '#f59e0b';
            } else {
                count.style.color = '';
            }
        }
        
        function updateCommentCount() {
            const input = document.getElementById('commentInput');
            const count = document.getElementById('commentCount');
            count.textContent = input.value.length + '/256';
            if (input.value.length > 220) {
                count.style.color = '#f59e0b';
            } else {
                count.style.color = '';
            }
        }
        
        async function createTicket() {
            const title = document.getElementById('ticketTitle').value.trim();
            if (!title) {
                alert("Title is required");
                return;
            }
            
            const description = document.getElementById('ticketDesc').value.trim();
            const priority = document.getElementById('ticketPriority').value;
            const assignee = document.getElementById('ticketAssignee').value.trim(); // Empty if not set - allows goban-cli to claim unassigned tickets
            const dueDate = document.getElementById('ticketDueDate').value;
            const labelsStr = document.getElementById('ticketLabels').value.trim();
            let labels = [];
            if (labelsStr) {
                labels = labelsStr.split(',').map(l => l.trim()).filter(l => l);
            }
            
            try {
                const res = await fetch('/api/tickets', {
                    method: 'POST',
                    headers: { 
                        'Content-Type': 'application/json',
                        ...getAuthHeaders()
                    },
                    body: JSON.stringify({
                        title: title,
                        description: description,
                        priority: priority,
                        assignee: assignee,
                        due_date: dueDate,
                        labels: labels,
                        board_id: currentBoardId  // Create ticket on currently selected board
                    })
                });
                
                if (res.ok) {
                    hideNewTicketModal();
                    loadBoard(); // Refresh to show new ticket
                } else {
                    const err = await res.json().catch(() => ({}));
                    alert(err.error || 'Failed to create ticket');
                }
            } catch (e) {
                console.error("Error creating ticket:", e);
                alert("Error creating ticket");
            }
        }
        
        let currentDetailTicketId = null;
        
        function showTicketDetail(ticketId) {
            currentDetailTicketId = ticketId;
            
            // Find ticket in current data
            let found = null;
            for (let col of columnsData) {
                for (let t of col.tickets) {
                    if (t.id === ticketId) {
                        found = t;
                        break;
                    }
                }
                if (found) break;
            }
            
            if (!found) return;
            
            document.getElementById('detailTitle').textContent = found.title;
            document.getElementById('detailTicketId').textContent = found.id;
            document.getElementById('editTitle').value = found.title;
            document.getElementById('editAssignee').value = found.assignee || '';
            document.getElementById('editPriority').value = found.priority || 'medium';
            document.getElementById('editDueDate').value = found.due_date || '';
            document.getElementById('detailDescriptionEdit').value = found.description || '';
            document.getElementById('editLabels').value = (found.labels || []).join(', ');
            
            // Show/hide Force Archive button based on status and user role
            const forceArchiveBtn = document.getElementById('forceArchiveBtn');
            if (['DONE', 'REVIEW'].includes(found.status) && currentUserData?.role === 'HUMAN_ADMIN') {
                forceArchiveBtn.style.display = 'block';
            } else {
                forceArchiveBtn.style.display = 'none';
            }
            
            // Render activity log
            renderActivityLog(found.comments || []);
            
            // Render subtasks
            renderSubtasks(found.subtasks || []);
            
            document.getElementById('ticketDetailModal').classList.remove('hidden');

            // Show/hide Release to Pool button based on whether ticket has an assignee
            const releaseBtn = document.getElementById('releasePoolButton');
            if (found.assignee && found.assignee.trim() !== '') {
                releaseBtn.style.display = '';  // Show button for assigned tickets
            } else {
                releaseBtn.style.display = 'none';  // Hide for unassigned tickets
            }
        }
        
        function renderActivityLog(comments) {
            const container = document.getElementById('activityLog');
            container.innerHTML = '';
            
            if (!comments || comments.length === 0) {
                container.innerHTML = '<div class="text-gray-500 text-xs italic">No activity yet.</div>';
                return;
            }
            
            comments.forEach(comment => {
                const div = document.createElement('div');
                div.className = 'bg-gray-800 rounded-2xl p-4 text-sm';
                div.innerHTML = `
                    <div class="flex justify-between text-xs text-gray-500 mb-1">
                        <span>${escapeHtml(comment.who)}</span>
                        <span>${escapeHtml(comment.timestamp || 'just now')}</span>
                    </div>
                    <div class="text-gray-300">${escapeHtml(comment.text)}</div>
                `;
                container.appendChild(div);
            });
        }
        
        async function postComment() {
            const input = document.getElementById('commentInput');
            const text = input.value.trim();
            if (!text) return;
            
            if (!currentDetailTicketId) return;
            
            const who = document.getElementById('currentUser').value.trim() || "user";
            
            const comment = {
                who: who,
                text: text,
                timestamp: new Date().toLocaleTimeString([], {hour:'2-digit', minute:'2-digit'})
            };
            
            // Find ticket and add comment locally
            let targetTicket = null;
            for (let col of columnsData) {
                for (let t of col.tickets) {
                    if (t.id === currentDetailTicketId) {
                        if (!t.comments) t.comments = [];
                        t.comments.unshift(comment);
                        targetTicket = t;
                        break;
                    }
                }
                if (targetTicket) break;
            }
            
            if (targetTicket) {
                renderActivityLog(targetTicket.comments);
                input.value = '';
                
                // Persist to backend using dedicated comments endpoint (ticket-55059)
                try {
                    const response = await fetch('/api/comments/' + currentDetailTicketId, {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({
                            text: text,
                            who: who
                        })
                    });
                    
                    if (!response.ok) {
                        const error = await response.json();
                        console.error("Failed to persist comment:", error);
                        // Revert local change on failure
                        targetTicket.comments.shift();
                        renderActivityLog(targetTicket.comments);
                        alert('Comment not saved: ' + (error.error || 'Unknown error'));
                    }
                } catch (e) {
                    console.error("Failed to persist comment:", e);
                    // Revert local change on network error  
                    targetTicket.comments.shift();
                    renderActivityLog(targetTicket.comments);
                }
            }
        }
        
        function hideTicketDetail() {
            document.getElementById('ticketDetailModal').classList.add('hidden');
            currentDetailTicketId = null;
        }

        // Subtask management functions
        let subtasksData = [];

        function renderSubtasks(subtasks) {
            subtasksData = subtasks || [];
            const container = document.getElementById('subtaskList');
            container.innerHTML = '';

            if (subtasksData.length === 0) {
                container.innerHTML = '<div class="text-gray-500 text-xs italic">No subtasks yet. Add one above.</div>';
                updateSubtaskProgress();
                return;
            }

            subtasksData.forEach((st, index) => {
                const div = document.createElement('div');
                div.className = 'bg-gray-800 rounded-2xl p-3 flex items-center gap-3';
                div.innerHTML = `
                    <input type="checkbox" ${st.completed ? 'checked' : ''} 
                           onchange="toggleSubtask(${index})"
                           class="w-5 h-5 rounded border-gray-600 bg-gray-700 text-blue-600 focus:ring-offset-gray-800">
                    <span class="flex-1 ${st.completed ? 'text-gray-500 line-through' : 'text-white'}">${escapeHtml(st.title)}</span>
                    <button onclick="deleteSubtask(${index})" 
                            class="text-red-400 hover:text-red-300 text-sm px-2">×</button>
                `;
                container.appendChild(div);
            });

            updateSubtaskProgress();
        }

        function addSubtask() {
            const input = document.getElementById('newSubtaskInput');
            const title = input.value.trim();
            if (!title) return;

            subtasksData.push({
                title: title,
                completed: false
            });

            renderSubtasks(subtasksData);
            input.value = '';
        }

        function toggleSubtask(index) {
            subtasksData[index].completed = !subtasksData[index].completed;
            renderSubtasks(subtasksData);
        }

        function deleteSubtask(index) {
            subtasksData.splice(index, 1);
            renderSubtasks(subtasksData);
        }

        function updateSubtaskProgress() {
            const completed = subtasksData.filter(st => st.completed).length;
            const total = subtasksData.length;
            document.getElementById('subtaskProgress').textContent = `${completed}/${total}`;
        }

        async function saveTicketChanges() {
            if (!currentDetailTicketId) return;
            
            const title = document.getElementById('editTitle').value.trim();
            const assignee = document.getElementById('editAssignee').value.trim();
            const priority = document.getElementById('editPriority').value;
            const dueDate = document.getElementById('editDueDate').value;
            const labelsInput = document.getElementById('editLabels').value.trim();
            const description = document.getElementById('detailDescriptionEdit').value.trim();
            const subtasks = [...subtasksData]; // send copy
            
            // Parse labels from comma-separated string
            const labels = labelsInput ? labelsInput.split(',').map(l => l.trim()).filter(l => l) : [];
            
            try {
                const res = await fetch('/api/tickets/' + currentDetailTicketId, {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        title: title,
                        assignee: assignee,
                        priority: priority,
                        due_date: dueDate,
                        labels: labels,
                        description: description,
                        subtasks: subtasks
                    })
                });
                
                if (res.ok) {
                    hideTicketDetail();
                    loadBoard();
                } else {
                    const err = await res.json().catch(() => ({}));
                    alert(err.error || 'Failed to save changes');
                }
            } catch (e) {
                console.error(e);
                alert("Error saving changes");
            }
        }
        
        async function deleteCurrentTicket() {
            if (!currentDetailTicketId) return;
            if (!confirm("Delete this ticket permanently?")) return;
            
            try {
                const res = await fetch('/api/tickets/' + currentDetailTicketId, {
                    method: 'DELETE',
                    ...getAuthHeaders()
                });
                
                if (res.ok) {
                    hideTicketDetail();
                    loadBoard();
                } else {
                    const err = await res.json().catch(() => ({}));
                    alert(err.error || 'Failed to delete ticket');
                }
            } catch (e) {
                console.error("Delete error:", e);
                alert("Error deleting ticket");
            }
        }

        async function releaseCurrentTicket() {
            if (!currentDetailTicketId) return;
            const ticket = columnsData.flatMap(c => c.tickets).find(t => t.id === currentDetailTicketId);
            if (!ticket || !ticket.assignee) return;
            if (!confirm(`Release "${ticket.title}" from ${ticket.assignee}?`)) return;

            try {
                const res = await fetch('/api/tickets/' + currentDetailTicketId + '/release', {
                    method: 'POST'
                });
                if (res.ok) {
                    hideTicketDetail();
                    loadBoard();
                } else {
                    const err = await res.json();
                    alert('Release failed: ' + (err.message || 'Unknown error'));
                }
            } catch (e) {
                console.error("Release error:", e);
                alert("Error releasing ticket");
            }
        }

        // ============================================
        // Force Archive Handlers (ticket: ab7d4392f8)
        // ============================================
        
        async function forceArchiveTicket() {
            if (!currentDetailTicketId) return;
            if (!currentUserData || !currentUserData.id) {
                alert("Please login as admin first");
                showLoginModal();
                return;
            }
            
            if (!confirm(`Force archive ticket ${currentDetailTicketId}? This will move it to archived state.`)) return;
            
            try {
                const res = await fetch('/api/archive/single', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        ...getAuthHeaders()
                    },
                    body: JSON.stringify({ ticket_id: currentDetailTicketId })
                });
                
                const data = await res.json();
                
                if (res.ok) {
                    alert("Ticket archived successfully");
                    hideTicketDetail();
                    loadBoard();
                } else {
                    alert(`Failed to archive ticket: ${data.error || 'Unknown error'}`);
                }
            } catch (e) {
                console.error("Archive error:", e);
                alert("Error archiving ticket");
            }
        }
        
        async function bulkArchiveTickets() {
            // Collect all ticket IDs in DONE or CANCELLED columns
            const ticketIds = [];
            for (const col of columnsData) {
                if (['DONE', 'CANCELLED'].includes(columnIdToStatus(col.id))) {
                    (col.tickets || []).forEach(t => ticketIds.push(t.id));
                }
            }

            if (ticketIds.length === 0) {
                alert("No tickets in DONE or CANCELLED columns to archive");
                return;
            }

            if (!currentUserData || !currentUserData.id) {
                alert("Please login as admin first");
                showLoginModal();
                return;
            }

            if (!confirm(`Archive ${ticketIds.length} tickets in DONE/CANCELLED columns?`)) return;

            try {
                const res = await fetch(`/api/archive/bulk`, {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        ...getAuthHeaders()
                    },
                    body: JSON.stringify({ ticket_ids: ticketIds })
                });

                const data = await res.json();

                if (res.ok) {
                    const msg = data.not_found && data.not_found.length > 0
                        ? `${data.count} archived, ${data.not_found.length} not found: ${data.not_found.join(', ')}`
                        : `${data.count || ticketIds.length} tickets archived successfully`;
                    alert(msg);
                    loadBoard();
                } else {
                    alert(`Failed to archive tickets: ${data.error || 'Unknown error'}`);
                }
            } catch (e) {
                console.error("Bulk archive error:", e);
                alert("Error archiving tickets");
            }
        }
        
        // ============================================
        // Board Selection Persistence (ticket: 52ddfb708e14c507)
        // ============================================
        
        const BOARD_STORAGE_KEY = 'goban_selected_board';
        
        function loadSavedBoard() {
            try {
                return localStorage.getItem(BOARD_STORAGE_KEY);
            } catch (e) {
                console.warn('localStorage not available for board persistence:', e);
                return null;
            }
        }
        
        function saveSelectedBoard(boardId) {
            if (!boardId) return;
            try {
                localStorage.setItem(BOARD_STORAGE_KEY, boardId);
            } catch (e) {
                console.warn('Failed to save selected board to localStorage:', e);
            }
        }
        
        // ============================================
        // Username Persistence (tickets: 15aba03f89, afd7ea5ebe, 512f76427b, eaa4954582)
        // ============================================
        
        const USERNAME_STORAGE_KEY = 'goban_username';
        const MAX_USERNAME_LENGTH = 32;
        let usernameSaveTimeout = null;
        
        function loadSavedUsername() {
            try {
                const savedUsername = localStorage.getItem(USERNAME_STORAGE_KEY);
                if (savedUsername) {
                    // Truncate to max length for safety (defense against old data)
                    document.getElementById('currentUser').value = savedUsername.substring(0, MAX_USERNAME_LENGTH);
                }
                updateNameCharCounter();
            } catch (e) {
                console.warn('localStorage not available for username persistence:', e);
            }
        }
        
        function saveUsername() {
            const input = document.getElementById('currentUser');
            let value = input.value.trim();
            
            // Enforce max length on save as well (defense against paste bypass)
            if (value.length > MAX_USERNAME_LENGTH) {
                value = value.substring(0, MAX_USERNAME_LENGTH);
                input.value = value;
            }
            
            try {
                localStorage.setItem(USERNAME_STORAGE_KEY, value);
            } catch (e) {
                console.warn('Failed to save username to localStorage:', e);
            }
        }
        
        function updateNameCharCounter() {
            const input = document.getElementById('currentUser');
            const counterEl = document.getElementById('nameCharCounter');
            const length = input.value.length;
            
            if (length === 0) {
                counterEl.classList.add('hidden');
                return;
            }
            
            counterEl.textContent = `${length}/${MAX_USERNAME_LENGTH}`;
            counterEl.classList.remove('hidden');
            
            // Color coding: green < 80%, yellow >= 80%, red > 100% (shouldn't happen but defensive)
            const percentage = length / MAX_USERNAME_LENGTH;
            if (percentage >= 0.8 && percentage <= 1.0) {
                counterEl.className = 'text-xs text-yellow-500'; // Warning color
            } else if (percentage > 1.0) {
                counterEl.className = 'text-xs text-red-500 font-bold'; // Error color
            } else {
                counterEl.className = 'text-xs text-gray-500'; // Normal
            }
        }
        
        function initUsernameField() {
            const input = document.getElementById('currentUser');
            
            // Load saved username on page load
            loadSavedUsername();
            
            // Debounced save on input change (wait 500ms after last keystroke)
            input.addEventListener('input', () => {
                updateNameCharCounter();
                clearTimeout(usernameSaveTimeout);
                usernameSaveTimeout = setTimeout(saveUsername, 500);
            });
            
            // Immediate save on blur (when user tabs/clicks away)
            input.addEventListener('blur', () => {
                saveUsername();
            });
            
            // Prevent paste of content exceeding max length
            input.addEventListener('paste', (e) => {
                e.preventDefault();
                const pastedText = e.clipboardData.getData('text');
                const currentValue = input.value;
                
                if ((currentValue + pastedText).length > MAX_USERNAME_LENGTH) {
                    // Truncate to fit within limit
                    const remainingSpace = MAX_USERNAME_LENGTH - currentValue.length;
                    if (remainingSpace > 0) {
                        input.value = currentValue + pastedText.substring(0, remainingSpace);
                    } else {
                        input.value = currentValue;
                    }
                } else {
                    input.value = currentValue + pastedText;
                }
                
                updateNameCharCounter();
                clearTimeout(usernameSaveTimeout);
                usernameSaveTimeout = setTimeout(saveUsername, 500);
            });
       }
        
        // ============================================
        // Authentication System
        // ============================================
        
        let authToken = null;
        let currentUserData = null;
        
        function showLoginModal() {
            document.getElementById('loginModal').classList.remove('hidden');
            document.getElementById('loginError').classList.add('hidden');
            document.getElementById('loginUsername').value = '';
            document.getElementById('loginPassword').value = '';
        }
        
        function hideLoginModal() {
            document.getElementById('loginModal').classList.add('hidden');
        }
        
        async function performLogin() {
            const username = document.getElementById('loginUsername').value.trim();
            const password = document.getElementById('loginPassword').value;
            
            if (!username || !password) {
                showLoginError('Please enter both username and password');
                return;
            }
            
            try {
                const response = await fetch('/api/auth/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username, password })
                });
                
                if (!response.ok) {
                    const err = await response.json();
                    showLoginError(err.message || 'Login failed');
                    return;
                }
                
                const data = await response.json();
                authToken = data.access_token;
                currentUserData = data.user;
                
                // Store token in localStorage
                try {
                    localStorage.setItem('goban_auth_token', authToken);
                } catch (e) {}
                
                // Update UI to show logged-in state
                updateAuthUI();
                hideLoginModal();
                
                // Refresh board with authenticated requests
                await loadBoard();
            } catch (err) {
                showLoginError('Connection error: ' + err.message);
            }
        }
        
        function showLoginError(message) {
            const el = document.getElementById('loginError');
            el.textContent = message;
            el.classList.remove('hidden');
        }
        
        async function logout() {
            authToken = null;
            currentUserData = null;
            
            try {
                localStorage.removeItem('goban_auth_token');
            } catch (e) {}
            
            updateAuthUI();
            await loadBoard();
        }
        
        function updateAuthUI() {
            const loginStatus = document.getElementById('loginStatus');
            const loginBtn = document.getElementById('loginBtn');
            const legacyUserField = document.getElementById('legacyUserField');
            const bulkArchiveBtn = document.getElementById('bulkArchiveBtn');
            
            if (authToken && currentUserData) {
                // Show logged-in state
                loginStatus.classList.remove('hidden');
                loginBtn.classList.add('hidden');
                legacyUserField.classList.add('hidden');
                
                document.getElementById('currentUserDisplay').textContent = currentUserData.name;
                
                const roleBadge = document.getElementById('userRoleBadge');
                roleBadge.textContent = currentUserData.role;
                roleBadge.classList.remove('hidden');
                
                // Show bulk archive button based on role
                if (currentUserData.role === 'HUMAN_ADMIN' || currentUserData.role === 'OVERSEER_AI') {
                    bulkArchiveBtn.classList.remove('hidden');
                } else {
                    bulkArchiveBtn.classList.add('hidden');
                }
            } else {
                // Show login button and legacy username field
                loginStatus.classList.add('hidden');
                loginBtn.classList.remove('hidden');
                legacyUserField.classList.remove('hidden');
                bulkArchiveBtn.classList.add('hidden');
            }
        }
        
        function loadSavedAuth() {
            try {
                const savedToken = localStorage.getItem('goban_auth_token');
                if (savedToken) {
                    authToken = savedToken;
                    // Verify token is still valid by checking /me endpoint
                    fetch('/api/auth/me', {
                        headers: { 'Authorization': 'Bearer ' + authToken }
                    }).then(async (resp) => {
                        if (resp.ok) {
                            currentUserData = await resp.json();
                            updateAuthUI();
                        } else {
                            logout();
                        }
                    }).catch(() => logout());
                }
            } catch (e) {}
        }
        
        function getAuthHeaders() {
            if (!authToken) return {};
            return { 'Authorization': 'Bearer ' + authToken };
        }
        
        // Initial load
        window.onload = () => {
            initSSE(); // Initialize SSE for real-time updates
            loadSavedAuth(); // Load saved authentication token
            initUsernameField(); // Initialize username persistence (tickets: 15aba03f89, afd7ea5ebe, 512f76427b, eaa4954582)
            loadBoard();
            
            // Keep polling as fallback (every 30 seconds) when SSE is not available
            setInterval(() => {
                if (!sseConnected) {
                    loadBoard();
                }
            }, 30000);
        };
    