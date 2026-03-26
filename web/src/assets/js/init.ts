// Load vendor scripts
import '../vendor/js/helpers.js';
import '../vendor/js/menu.js';

// Declare global types for the vendor libraries
declare global {
  interface Window {
    Menu: any;
    Helpers: any;
    PerfectScrollbar: any;
  }
}

/**
 * Initialize menu and layout helpers
 */
export function initializeLayout() {
  // Wait for DOM to be ready
  if (typeof window === 'undefined' || !window.Helpers || !window.Menu) {
    console.warn('Helpers or Menu not available yet');
    return;
  }

  // Initialize Helpers
  if (!window.Helpers._initialized) {
    window.Helpers.init();
    window.Helpers.initPasswordToggle();
    window.Helpers.setAutoUpdate(true);
  }

  // Initialize Menu
  const menuEl = document.querySelector('#layout-menu');
  if (menuEl) {
    try {
      // Check if menu is already initialized
      if (!(menuEl as any).menuInstance) {
        const menu = new window.Menu(menuEl, {
          animate: true,
          accordion: true,
        });

        window.Helpers.mainMenu = menu;

        // Scroll to active menu item
        if (window.Helpers.scrollToActive) {
          window.Helpers.scrollToActive(true);
        }
      }
    } catch (error) {
      console.error('Error initializing menu:', error);
    }
  }

  // Initialize layout menu toggle for mobile
  initLayoutMenuToggle();
}

/**
 * Initialize layout menu toggle button for mobile devices
 */
function initLayoutMenuToggle() {
  const togglers = document.querySelectorAll('.layout-menu-toggle');

  togglers.forEach((toggler) => {
    toggler.addEventListener('click', (e) => {
      e.preventDefault();

      if (window.Helpers) {
        window.Helpers.toggleCollapsed();
      }
    });
  });

  // Close menu when clicking overlay on mobile
  const overlay = document.querySelector('.layout-overlay');
  if (overlay) {
    overlay.addEventListener('click', (e) => {
      e.preventDefault();

      if (window.Helpers && !window.Helpers.isCollapsed()) {
        window.Helpers.setCollapsed(true);
      }
    });
  }
}

/**
 * Cleanup function for when component is unmounted
 */
export function cleanupLayout() {
  if (window.Helpers && window.Helpers._initialized) {
    window.Helpers.destroy();
  }

  const menuEl = document.querySelector('#layout-menu');
  if (menuEl && (menuEl as any).menuInstance) {
    (menuEl as any).menuInstance.destroy();
  }
}
