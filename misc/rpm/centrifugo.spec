%define debug_package %{nil}
%define application_user  %{name}
%define application_group %{name}
%define application_data  %{_localstatedir}/lib/%{name}
%define __prefix /opt

Name:      centrifugo
Version:   %{version}
Release:   %{release}
Summary:   Real-time messaging server
License:   MIT
URL:       https://github.com/centrifugal/centrifugo
Group:     Apps/sys
Buildroot: %{_tmppath}/%{name}-%{version}-%{release}-%(%{__id_u} -n)
Packager:  Alexandr Emelin <frvzmb@gmail.com>
Requires(pre): shadow-utils
Requires(post): /sbin/chkconfig
Requires(preun): /sbin/chkconfig, /sbin/service
Requires(postun): /sbin/service

%description
Real-time messaging server written in Go

%prep
cd ~/rpmbuild/BUILD
rm -rf %{name}-%{version}-linux-amd64
unzip ~/rpmbuild/SOURCES/%{name}-%{version}-linux-amd64.zip
if [ $? -ne 0 ]; then
  exit $?
fi
ls
cd %{name}-%{version}-linux-amd64
chown -R $USER.$USER .
chmod -R a+rX,g-w,o-w .

%build
rm -rf %{buildroot}

echo  %{buildroot}

%install
install -d -m 755 %{buildroot}/%{_sbindir}
install    -m 755 %{_builddir}/%{name}-%{version}-linux-amd64/centrifugo %{buildroot}/%{_sbindir}

install -d -m 755 %{buildroot}%{__prefix}/%{name}

install -d -m 755 %{buildroot}/%{_localstatedir}/log/%{name}
install -d -m 755 %{buildroot}/%{_localstatedir}/lib/%{name}

install -d -m 755 %{buildroot}/%{_initrddir}
install    -m 755 %_sourcedir/%{name}.initd %{buildroot}/%{_initrddir}/%{name}

install -d -m 755 %{buildroot}/%{_sysconfdir}/security/limits.d/
install    -m 644 %_sourcedir/%{name}.nofiles.conf %{buildroot}/%{_sysconfdir}/security/limits.d/%{name}.nofiles.conf

install -d -m 755 %{buildroot}/%{_sysconfdir}/logrotate.d
install    -m 644 %_sourcedir/%{name}.logrotate %{buildroot}/%{_sysconfdir}/logrotate.d/%{name}

install -d -m 755 %{buildroot}/%{_sysconfdir}/%{name}/
install    -m 644 %_sourcedir/%{name}.config.json %{buildroot}/%{_sysconfdir}/%{name}/config.json

%clean
rm -rf %{buildroot}

%pre
getent group %{application_group} >/dev/null || groupadd -r %{application_group}
getent passwd %{application_user} >/dev/null || /usr/sbin/useradd --comment "centrifugo Daemon User" --shell /bin/bash -M -r -g %{application_group} --home %{application_data} %{application_user}

%post
chkconfig --add %{name}

%preun
if [ $1 = 0 ]; then
  service %{name} stop > /dev/null 2>&1 || :
  chkconfig --del %{name}
fi

%postun
if [ $1 == 1 ] ; then
  service %{name} condrestart >/dev/null 2>&1 || :
fi

%files
%defattr(-,root,root)
%{_sbindir}/%{name}
%attr(0755,%{application_user},%{application_group}) %dir %{_localstatedir}/log/%{name}
%attr(0755,%{application_user},%{application_group}) %dir %{_localstatedir}/lib/%{name}
%{_initrddir}/%{name}
%{__prefix}/%{name}
%config(noreplace) %{_sysconfdir}/%{name}/config.json
%config(noreplace) %{_sysconfdir}/security/limits.d/%{name}.nofiles.conf
%config(noreplace) %{_sysconfdir}/logrotate.d/%{name}
