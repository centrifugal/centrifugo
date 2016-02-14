#!/bin/sh
make prepare
gem install package_cloud
gem install fpm
sudo apt-get install -y rpm
make packages
make packagecloud
