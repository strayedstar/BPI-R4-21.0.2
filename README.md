Fork from BPI-SINOVOIP/BPI-R4-OPENWRT-V21.02
BPI-R4-OPENWRT-V21.02 The BSP don't include any wifi driver and don't support BE19000, BE13500 WiFi Card.
Development
To build your own firmware you need a GNU/Linux, BSD or MacOSX system (case sensitive filesystem required). Cygwin is unsupported because of the lack of a case sensitive file system.

Requirements
You need the following tools to compile OpenWrt, the package names vary between distributions. A complete list with distribution specific packages is found in the Build System Setup documentation.

gcc binutils bzip2 flex python3 perl make find grep diff unzip gawk getopt
subversion libz-dev libc-dev rsync which
Quickstart
Run ./scripts/feeds update -a to obtain all the latest package definitions defined in feeds.conf / feeds.conf.default

Run ./scripts/feeds install -a to install symlinks for all obtained packages into package/feeds/

Run make menuconfig to select your preferred configuration for the toolchain, target system & firmware packages.

Run make to build your firmware. This will download all sources, build the cross-compile toolchain and then cross-compile the GNU/Linux kernel & all chosen applications for your target system.

OpenWrt is licensed under GPL-2.0

可以较为正常的构建镜像,但是EMMC和SD卡依然会有jffs2报错(一段时间以后正常,正常以前对文件系统的提交会被延后.),推测可能是rootfs或者GPT-image的问题,但是没有时间解决,因为还要考试,欢迎有能力解决.
