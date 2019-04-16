# /!\ This file is maintained at https://github.com/openshift/prometheus-alertmanager
%global debug_package   %{nil}

%global provider        github
%global provider_tld    com
%global project         prometheus
%global repo            alertmanager
# https://github.com/prometheus/alertmanager
%global provider_prefix %{provider}.%{provider_tld}/%{project}/%{repo}
%global import_path     %{provider_prefix}
# %commit is intended to be set by tito. The values in this spec file will not be kept up to date.
%{!?commit:
%global commit          5e86f61bd73c6325d6049ab3dbcb468ede26dfe0
}
%global shortcommit     %(c=%{commit}; echo ${c:0:7})
%global gopathdir       %{_sourcedir}/go
%global upstream_ver    0.16.2
%global rpm_ver         %(v=%{upstream_ver}; echo ${v//-/_})
%global download_prefix %{provider}.%{provider_tld}/openshift/%{repo}

Name:		golang-%{provider}-%{project}-%{repo}
# Version and release information will be automatically managed by CD
# It will be kept in sync with OCP builds.
Version:	%{rpm_ver}
Release:	1.git%{shortcommit}%{?dist}
Summary:	The Prometheus Alertmanager handles alerts sent by client applications such as the Prometheus server.
License:	ASL 2.0
URL:		https://prometheus.io/
Source0:	https://%{download_prefix}/archive/%{commit}/%{repo}-%{commit}.tar.gz

# e.g. el6 has ppc64 arch without gcc-go, so EA tag is required
ExclusiveArch:  %{?go_arches:%{go_arches}}%{!?go_arches:%{ix86} x86_64 aarch64 %{arm} ppc64le s390x}
# If go_compiler is not set to 1, there is no virtual provide. Use golang instead.
BuildRequires: %{?go_compiler:compiler(go-compiler)}%{!?go_compiler:golang}
BuildRequires: glibc-static
BuildRequires: prometheus-promu

%description
%{summary}

%package -n %{project}-alertmanager
Summary:        %{summary}
Provides:       prometheus-alertmanager = %{version}-%{release}
Obsoletes:      prometheus-alertmanager < %{version}-%{release}

%description -n %{project}-alertmanager
%{summary}

%prep
%setup -q -n %{repo}-%{commit}

%build
# Go expects a full path to the sources which is not included in the source
# tarball so create a link with the expected path
mkdir -p %{gopathdir}/src/%{provider}.%{provider_tld}/%{project}
GOSRCDIR=%{gopathdir}/src/%{import_path}
if [ ! -e "$GOSRCDIR" ]; then
  ln -s `pwd` "$GOSRCDIR"
fi
export GOPATH=%{gopathdir}

make build BUILD_PROMU=false

%install
install -d %{buildroot}%{_bindir}
install -D -p -m 0755 alertmanager %{buildroot}%{_bindir}/alertmanager
install -D -p -m 0755 amtool %{buildroot}%{_bindir}/amtool
install -D -p -m 0644 doc/examples/simple.yml \
                      %{buildroot}%{_sysconfdir}/prometheus/alertmanager.yml
install -D -p -m 0644 prometheus-alertmanager.service \
                      %{buildroot}%{_unitdir}/prometheus-alertmanager.service
install -D -p -m 0644 prometheus-alertmanager.sysconfig \
                      %{buildroot}%{_sysconfdir}/sysconfig/prometheus-alertmanager
install -d %{buildroot}%{_sharedstatedir}/prometheus-alertmanager

%files -n %{project}-alertmanager
%license LICENSE NOTICE
%doc CHANGELOG.md CONTRIBUTING.md MAINTAINERS.md README.md
%{_bindir}/alertmanager
%{_bindir}/amtool
%{_sysconfdir}/prometheus/alertmanager.yml
%{_unitdir}/prometheus-alertmanager.service
%{_sysconfdir}/sysconfig/prometheus-alertmanager
%{_sharedstatedir}/prometheus-alertmanager

%changelog
* Fri Apr 5 2019 Simon Pasquier <spasquie@redhat.com> - 0.16.2-1
- Upgrade to 0.16.2

* Mon Jan 21 2019 Simon Pasquier <spasquie@redhat.com> - 0.16.0-1
- Upgrade to 0.16.0

* Tue Nov 13 2018 Paul Gier <pgier@redhat.com> - 0.15.3-1
- Upgrade to 0.15.3

* Thu Sep 27 2018 Simon Pasquier <spasqui@redhat.com> - 0.15.2-2
- Fix stop command in systemd unit

* Tue Aug 14 2018 Simon Pasquier <spasqui@redhat.com> - 0.15.2-1
- Upgrade to 0.15.2

* Fri Jul 27 2018 Simon Pasquier <spasqui@redhat.com> - 0.15.1-2
- Enable aarch64

* Mon Jul 16 2018 Simon Pasquier <spasqui@redhat.com> - 0.15.1-1
- Upgrade to 0.15.1

* Mon Jun 18 2018 Simon Pasquier <spasqui@redhat.com> - 0.15.0-1
- Upgrade to 0.15.0

* Tue Feb 13 2018 Simon Pasquier <spasqui@redhat.com> - 0.14.0-1
- Upgrade to 0.14.0

* Wed Jan 31 2018 Paul Gier <pgier@redhat.com> - 0.13.0-1
- Upgrade to 0.13.0 and include amtool

* Thu Jan 18 2018 Simon Pasquier <spasquie@redhat.com> - 0.12.0-1
- Upgrade to 0.12.0

* Wed Jan 10 2018 Yaakov Selkowitz <yselkowi@redhat.com> - 0.9.1-3
- Rebuilt for ppc64le, s390x enablement

* Wed Nov 08 2017 Paul Gier <pgier@redhat.com> - 0.9.1-2
- upgrade to 0.9.1

* Fri Sep 01 2017 Paul Gier <pgier@redhat.com> - 0.8.0-2
- Strip debug and remove duplicate package

* Fri Aug 25 2017 Paul Gier <pgier@redhat.com> - 0.8.0-1
- First package for Openshift
