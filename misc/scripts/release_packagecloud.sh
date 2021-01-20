#!/bin/sh
sudo gem install package_cloud
sudo gem install fpm
sudo apt-get install -y rpm
make package
#make packagecloud
