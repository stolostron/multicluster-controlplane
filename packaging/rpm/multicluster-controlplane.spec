# parameters that must be provided via --define , or fixed into the spec file:
# global version 1.0.0
# global release 2023_02_02_00001
# global commit 81264d0ebb17fef06eff9ec7d4f2a81631c6b34a

# git related details
%global shortcommit %(c=%{commit}; echo ${c:0:7})

# golang specifics
%global golang_version 1.19
#debuginfo not supported with Go
%global debug_package %{nil}
# modifying the Go binaries breaks the DWARF debugging
%global __os_install_post %{_rpmconfigdir}/brp-compress

Name: multicluster-controlplane
Version: %{version}
Release: %{release}%{dist}
Summary: Multicluster ControlPlane Service
License: Apache-2.0
URL: https://github.com/stolostron/multicluster-controlplane
Source0: https://github.com/stolostron/multicluster-controlplane/archive/%{commit}/multicluster-controlplane-%{shortcommit}.tar.gz

#ExclusiveArch: x86_64 aarch64

BuildRequires: golang >= %{golang_version}
BuildRequires: make
BuildRequires: systemd

# for generating certificates
Requires: sed
Requires: openssl

%{?systemd_requires}

%description
The multicluster-controlplane package provides a standalone controlplane to run ocm core.

%prep
%setup -n multicluster-controlplane-%{commit}

%build

make build

%install

install -d %{buildroot}%{_bindir}
install -p -m755 hack/lib/util.sh %{buildroot}%{_bindir}/multicluster-controlplane-pre-install-util.sh
install -p -m755 packaging/multicluster-controlplane/pre-install.sh %{buildroot}%{_bindir}/multicluster-controlplane-pre-install.sh
install -p -m755 ./bin/multicluster-controlplane %{buildroot}%{_bindir}/multicluster-controlplane

install -d -m755 %{buildroot}/%{_unitdir}
install -p -m644 packaging/systemd/multicluster-controlplane.service %{buildroot}%{_unitdir}/multicluster-controlplane.service

#install -d -m755 %{buildroot}/%{_sysconfdir}/multicluster-controlplane
#install -p -m644 packaging/multicluster-controlplane/config.yaml %{buildroot}%{_sysconfdir}/multicluster-controlplane/config.yaml.default

%post

%{_bindir}/multicluster-controlplane-pre-install.sh

%systemd_post multicluster-controlplane.service

%preun

%systemd_preun multicluster-controlplane.service

%files

%license LICENSE
%{_bindir}/multicluster-controlplane-pre-install-util.sh
%{_bindir}/multicluster-controlplane-pre-install.sh
%{_bindir}/multicluster-controlplane
%{_unitdir}/multicluster-controlplane.service
#%config(noreplace) %{_sysconfdir}/multicluster-controlplane/config.yaml.default

%changelog
* Thu Feb 02 2023 Morven Cao <lcao@redhat.com> 1.0.0
- Initial packaging
