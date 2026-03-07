<script lang="ts" setup>
import leftNavBar from "./leftNavBar.vue";
import themeSelector from "./themeSelector.vue";

import "../assets/vendor/fonts/iconify-icons.css";
import "../assets/vendor/css/core.css";
import "../assets/css/demo.css";
import "../assets/vendor/libs/perfect-scrollbar/perfect-scrollbar.css";
import "../assets/css/layout_override.css";

const currentYear = new Date().getFullYear();
</script>

<template>
  <div class="layout-wrapper layout-content-navbar">
    <div class="layout-container">
      <!-- Menu -->

      <left-nav-bar />
      <!-- / Menu -->

      <!-- Layout container -->
      <div class="layout-page">
        <!-- Navbar -->

        <nav
          class="layout-navbar container-fluid navbar-detached navbar navbar-expand-xl align-items-center bg-navbar-theme"
          id="layout-navbar"
        >
          <div
            class="layout-menu-toggle navbar-nav align-items-xl-center me-4 me-xl-0 d-xl-none"
          >
            <a class="nav-item nav-link px-0 me-xl-6" href="javascript:void(0)">
              <i class="icon-base bx bx-menu icon-md"></i>
            </a>
          </div>

          <div
            class="navbar-nav-right d-flex align-items-center justify-content-end"
            id="navbar-collapse"
          >
            <!-- Search -->
            <div class="navbar-nav align-items-center me-auto">
              <div class="nav-item d-flex align-items-center">
                <span class="w-px-22 h-px-22"
                  ><i class="icon-base bx bx-search icon-md"></i
                ></span>
                <input
                  type="text"
                  class="form-control border-0 shadow-none ps-1 ps-sm-2 d-md-block d-none"
                  placeholder="Search..."
                  aria-label="Search..."
                />
              </div>
            </div>
            <!-- /Search -->

            <ul class="navbar-nav flex-row align-items-center ms-md-auto">
              <!-- Place this tag where you want the button to render. -->
              <theme-selector />

              <!-- User -->
              <li class="nav-item navbar-dropdown dropdown-user dropdown">
                <a
                  class="nav-link dropdown-toggle hide-arrow p-0"
                  href="javascript:void(0);"
                  data-bs-toggle="dropdown"
                >
                  <div class="avatar avatar-online">
                    <img
                      src="../assets/img/avatars/1.png"
                      class="w-px-40 h-auto rounded-circle"
                    />
                  </div>
                </a>
                <ul class="dropdown-menu dropdown-menu-end">
                  <li>
                    <a class="dropdown-item" href="#">
                      <div class="d-flex">
                        <div class="flex-shrink-0 me-3">
                          <div class="avatar avatar-online">
                            <img
                              src="../assets/img/avatars/1.png"
                              class="w-px-40 h-auto rounded-circle"
                            />
                          </div>
                        </div>
                        <div class="flex-grow-1">
                          <h6 class="mb-0">John Doe</h6>
                          <small class="text-body-secondary">Admin</small>
                        </div>
                      </div>
                    </a>
                  </li>
                  <li>
                    <div class="dropdown-divider my-1"></div>
                  </li>
                  <li>
                    <a class="dropdown-item" href="#">
                      <i class="icon-base bx bx-user icon-md me-3"></i
                      ><span>My Profile</span>
                    </a>
                  </li>
                  <li>
                    <a class="dropdown-item" href="#">
                      <i class="icon-base bx bx-cog icon-md me-3"></i
                      ><span>Settings</span>
                    </a>
                  </li>
                  <li>
                    <a class="dropdown-item" href="#">
                      <span class="d-flex align-items-center align-middle">
                        <i
                          class="flex-shrink-0 icon-base bx bx-credit-card icon-md me-3"
                        ></i
                        ><span class="flex-grow-1 align-middle"
                          >Billing Plan</span
                        >
                        <span class="flex-shrink-0 badge rounded-pill bg-danger"
                          >4</span
                        >
                      </span>
                    </a>
                  </li>
                  <li>
                    <div class="dropdown-divider my-1"></div>
                  </li>
                  <li>
                    <a class="dropdown-item" href="javascript:void(0);">
                      <i class="icon-base bx bx-power-off icon-md me-3"></i
                      ><span>Log Out</span>
                    </a>
                  </li>
                </ul>
              </li>
              <!--/ User -->
            </ul>
          </div>
        </nav>

        <!-- / Navbar -->

        <!-- Content wrapper -->
        <div class="content-wrapper">
          <!-- Content -->
          <div class="container-fluid flex-grow-1 container-p-y">
            <RouterView />
          </div>
          <!-- / Content -->

          <!-- Footer -->
          <footer class="content-footer footer bg-footer-theme">
            <div class="container-fluid">
              <div
                class="footer-container d-flex align-items-center justify-content-between py-4 flex-md-row flex-column"
              >
                <div class="mb-2 mb-md-0">
                  &#169;
                  {{ currentYear }}, made with ❤️
                </div>
                <div class="d-none d-lg-inline-block">
                  <!-- <a
                    href="https://themeselection.com/item/category/admin-templates/"
                    target="_blank"
                    class="footer-link me-4"
                    >Admin Templates</a
                  >

                  <a
                    href="https://themeselection.com/license/"
                    class="footer-link me-4"
                    target="_blank"
                    >License</a
                  >
                  <a
                    href="https://themeselection.com/item/category/bootstrap-admin-templates/"
                    target="_blank"
                    class="footer-link me-4"
                    >Bootstrap Dashboard</a
                  >

                  <a
                    href="https://demos.themeselection.com/sneat-bootstrap-html-admin-template/documentation/"
                    target="_blank"
                    class="footer-link me-4"
                    >Documentation</a
                  >

                  <a
                    href="https://github.com/themeselection/sneat-bootstrap-html-admin-template-free/issues"
                    target="_blank"
                    class="footer-link"
                    >Support</a
                  > -->
                </div>
              </div>
            </div>
          </footer>
          <!-- / Footer -->

          <div class="content-backdrop fade"></div>
        </div>
        <!-- Content wrapper -->
      </div>
      <!-- / Layout page -->
    </div>

    <!-- Overlay -->
    <div class="layout-overlay layout-menu-toggle"></div>
  </div>
</template>
