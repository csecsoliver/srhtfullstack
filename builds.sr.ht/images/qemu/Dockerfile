FROM debian:trixie

RUN apt-get -y update --fix-missing && apt-get -y upgrade && apt-get install -y \
    build-essential pkg-config curl xz-utils

RUN curl -O https://download.qemu.org/qemu-10.0.0.tar.xz \
    && tar xvf qemu-10.0.0.tar.xz

RUN apt-get install -y \
    libaio-dev \
    libcap-dev \
    libcap-ng-dev \
    liblzo2-dev \
    texinfo \
    vde2 \
    zlib1g-dev \
    libpixman-1-dev \
    libglib2.0-dev \
    libzstd-dev \
    ninja-build \
    python3-venv \
    device-tree-compiler \
    libfdt-dev \
    libmount-dev \
    libslirp-dev

RUN cd qemu-10.0.0 && ./configure \
    --prefix=/ \
    --bindir=/usr/bin \
    --datadir=/share \
    --includedir=/include \
    --libdir=/usr/lib \
    --libexecdir=/libexec \
    --mandir=/share/man \
    --python=/usr/bin/python3 \
    --audio-drv-list="" \
    --enable-slirp \
    --disable-docs \
    --disable-debug-info \
    --disable-opengl \
    --disable-virglrenderer \
    --disable-vte \
    --disable-gtk \
    --disable-sdl \
    --disable-spice \
    --disable-vnc \
    --disable-curses \
    --disable-xen \
    --disable-smartcard \
    --disable-libnfs \
    --disable-libusb \
    --disable-glusterfs \
    --disable-gio \
    --disable-tools \
    --disable-werror \
    --target-list="x86_64-softmmu,i386-softmmu,aarch64-softmmu,arm-softmmu,ppc64-softmmu,s390x-softmmu,riscv64-softmmu,mips-softmmu,mipsel-softmmu,mips64el-softmmu"

RUN cd qemu-10.0.0 && make -j12 && make install
CMD ["/bin/qemu-system-x86_64"]
