// wlprobe.c -- minimal Wayland client that subscribes to wp_pointer_gestures_v1
// and prints pinch begin/update/end events for any pinch gesture made over
// the small window it opens.

#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <unistd.h>

#include <wayland-client.h>

#include "xdg-shell-client-protocol.h"
#include "pointer-gestures-client-protocol.h"

struct probe {
    struct wl_display *display;
    struct wl_registry *registry;
    struct wl_compositor *compositor;
    struct wl_shm *shm;
    struct wl_seat *seat;
    struct wl_pointer *pointer;
    struct xdg_wm_base *wm_base;
    struct zwp_pointer_gestures_v1 *gestures;
    struct zwp_pointer_gesture_pinch_v1 *pinch;

    struct wl_surface *surface;
    struct xdg_surface *xdg_surface;
    struct xdg_toplevel *xdg_toplevel;

    int width, height;
    int running;
    int configured;
};

// --- pinch listener --------------------------------------------------------

static void pinch_begin(void *data, struct zwp_pointer_gesture_pinch_v1 *p,
                        uint32_t serial, uint32_t time,
                        struct wl_surface *surface, uint32_t fingers) {
    (void)data; (void)p; (void)time; (void)surface;
    printf("pinch BEGIN  fingers=%u  serial=%u\n", fingers, serial);
    fflush(stdout);
}

static void pinch_update(void *data, struct zwp_pointer_gesture_pinch_v1 *p,
                         uint32_t time, wl_fixed_t dx, wl_fixed_t dy,
                         wl_fixed_t scale, wl_fixed_t rotation) {
    (void)data; (void)p; (void)time;
    printf("pinch UPDATE scale=%.3f  rotation=%6.2f deg  dx=%6.2f  dy=%6.2f\n",
           wl_fixed_to_double(scale),
           wl_fixed_to_double(rotation),
           wl_fixed_to_double(dx),
           wl_fixed_to_double(dy));
    fflush(stdout);
}

static void pinch_end(void *data, struct zwp_pointer_gesture_pinch_v1 *p,
                      uint32_t serial, uint32_t time, int32_t cancelled) {
    (void)data; (void)p; (void)time;
    printf("pinch END    cancelled=%d  serial=%u\n", cancelled, serial);
    fflush(stdout);
}

static const struct zwp_pointer_gesture_pinch_v1_listener pinch_listener = {
    .begin = pinch_begin,
    .update = pinch_update,
    .end = pinch_end,
};

// --- seat listener ---------------------------------------------------------

static void seat_capabilities(void *data, struct wl_seat *seat, uint32_t caps) {
    struct probe *p = data;
    if ((caps & WL_SEAT_CAPABILITY_POINTER) && !p->pointer) {
        p->pointer = wl_seat_get_pointer(seat);
        printf("✓ wl_pointer obtained from wl_seat\n");
        if (p->gestures) {
            p->pinch = zwp_pointer_gestures_v1_get_pinch_gesture(p->gestures, p->pointer);
            zwp_pointer_gesture_pinch_v1_add_listener(p->pinch, &pinch_listener, p);
            printf("✓ pinch gesture object attached — pinch over the window now\n");
        }
        fflush(stdout);
    }
}

static void seat_name(void *data, struct wl_seat *seat, const char *name) {
    (void)data; (void)seat; (void)name;
}

static const struct wl_seat_listener seat_listener = {
    .capabilities = seat_capabilities,
    .name = seat_name,
};

// --- xdg_wm_base ping ------------------------------------------------------

static void wm_base_ping(void *data, struct xdg_wm_base *b, uint32_t serial) {
    (void)data;
    xdg_wm_base_pong(b, serial);
}

static const struct xdg_wm_base_listener wm_base_listener = {
    .ping = wm_base_ping,
};

// --- shm buffer ------------------------------------------------------------

static struct wl_buffer *make_buffer(struct probe *p) {
    int stride = p->width * 4;
    int size = stride * p->height;
    int fd = memfd_create("wlprobe", MFD_CLOEXEC);
    if (fd < 0) { perror("memfd"); return NULL; }
    if (ftruncate(fd, size) < 0) { perror("ftruncate"); close(fd); return NULL; }
    uint32_t *data = mmap(NULL, size, PROT_READ|PROT_WRITE, MAP_SHARED, fd, 0);
    if (data == MAP_FAILED) { perror("mmap"); close(fd); return NULL; }
    for (int i = 0; i < p->width * p->height; i++) data[i] = 0xff442222; // ARGB
    munmap(data, size);
    struct wl_shm_pool *pool = wl_shm_create_pool(p->shm, fd, size);
    struct wl_buffer *buf = wl_shm_pool_create_buffer(pool, 0, p->width, p->height, stride, WL_SHM_FORMAT_ARGB8888);
    wl_shm_pool_destroy(pool);
    close(fd);
    return buf;
}

// --- xdg_surface configure -------------------------------------------------

static void xdg_surface_configure(void *data, struct xdg_surface *s, uint32_t serial) {
    struct probe *p = data;
    xdg_surface_ack_configure(s, serial);
    p->configured = 1;
    struct wl_buffer *buf = make_buffer(p);
    if (buf) {
        wl_surface_attach(p->surface, buf, 0, 0);
        wl_surface_damage_buffer(p->surface, 0, 0, p->width, p->height);
        wl_surface_commit(p->surface);
    }
}

static const struct xdg_surface_listener xdg_surface_listener = {
    .configure = xdg_surface_configure,
};

// --- xdg_toplevel listener -------------------------------------------------

static void toplevel_configure(void *data, struct xdg_toplevel *t,
                                int32_t w, int32_t h, struct wl_array *states) {
    (void)data; (void)t; (void)w; (void)h; (void)states;
}

static void toplevel_close(void *data, struct xdg_toplevel *t) {
    (void)t;
    struct probe *p = data;
    p->running = 0;
}

static void toplevel_configure_bounds(void *d, struct xdg_toplevel *t, int32_t w, int32_t h) {
    (void)d; (void)t; (void)w; (void)h;
}

static void toplevel_wm_capabilities(void *d, struct xdg_toplevel *t, struct wl_array *caps) {
    (void)d; (void)t; (void)caps;
}

static const struct xdg_toplevel_listener toplevel_listener = {
    .configure = toplevel_configure,
    .close = toplevel_close,
    .configure_bounds = toplevel_configure_bounds,
    .wm_capabilities = toplevel_wm_capabilities,
};

// --- registry --------------------------------------------------------------

static void registry_global(void *data, struct wl_registry *r, uint32_t name,
                            const char *iface, uint32_t version) {
    struct probe *p = data;
    if (strcmp(iface, wl_compositor_interface.name) == 0) {
        p->compositor = wl_registry_bind(r, name, &wl_compositor_interface, version > 4 ? 4 : version);
    } else if (strcmp(iface, wl_shm_interface.name) == 0) {
        p->shm = wl_registry_bind(r, name, &wl_shm_interface, 1);
    } else if (strcmp(iface, wl_seat_interface.name) == 0) {
        p->seat = wl_registry_bind(r, name, &wl_seat_interface, version > 7 ? 7 : version);
        wl_seat_add_listener(p->seat, &seat_listener, p);
    } else if (strcmp(iface, xdg_wm_base_interface.name) == 0) {
        p->wm_base = wl_registry_bind(r, name, &xdg_wm_base_interface, version > 4 ? 4 : version);
        xdg_wm_base_add_listener(p->wm_base, &wm_base_listener, p);
    } else if (strcmp(iface, zwp_pointer_gestures_v1_interface.name) == 0) {
        printf("✓ compositor advertises zwp_pointer_gestures_v1 (version %u)\n", version);
        fflush(stdout);
        p->gestures = wl_registry_bind(r, name, &zwp_pointer_gestures_v1_interface,
                                       version > 3 ? 3 : version);
    }
}

static void registry_global_remove(void *data, struct wl_registry *r, uint32_t name) {
    (void)data; (void)r; (void)name;
}

static const struct wl_registry_listener registry_listener = {
    .global = registry_global,
    .global_remove = registry_global_remove,
};

// --- entry point -----------------------------------------------------------

int wlprobe_run(void) {
    struct probe p = { .width = 320, .height = 240, .running = 1 };

    p.display = wl_display_connect(NULL);
    if (!p.display) {
        fprintf(stderr, "failed to connect to Wayland display ($WAYLAND_DISPLAY=%s)\n",
                getenv("WAYLAND_DISPLAY") ? getenv("WAYLAND_DISPLAY") : "(unset)");
        return 1;
    }

    p.registry = wl_display_get_registry(p.display);
    wl_registry_add_listener(p.registry, &registry_listener, &p);
    wl_display_roundtrip(p.display); // bring in globals
    wl_display_roundtrip(p.display); // process bind acks + initial events (e.g. seat caps)

    if (!p.compositor || !p.shm || !p.wm_base || !p.seat) {
        fprintf(stderr, "missing required globals\n");
        return 2;
    }
    if (!p.gestures) {
        fprintf(stderr, "✗ compositor does NOT advertise zwp_pointer_gestures_v1 — pinch will never fire\n");
    }

    p.surface = wl_compositor_create_surface(p.compositor);
    p.xdg_surface = xdg_wm_base_get_xdg_surface(p.wm_base, p.surface);
    xdg_surface_add_listener(p.xdg_surface, &xdg_surface_listener, &p);
    p.xdg_toplevel = xdg_surface_get_toplevel(p.xdg_surface);
    xdg_toplevel_add_listener(p.xdg_toplevel, &toplevel_listener, &p);
    xdg_toplevel_set_title(p.xdg_toplevel, "wl-pinch-probe");
    xdg_toplevel_set_app_id(p.xdg_toplevel, "wl-pinch-probe");
    wl_surface_commit(p.surface);

    printf("waiting for compositor configure …\n");
    printf("hover the cursor INTO the dark window and pinch on the touchpad\n");
    printf("Ctrl+C to quit\n");
    fflush(stdout);

    while (p.running && wl_display_dispatch(p.display) != -1) {}

    return 0;
}
