#!/bin/sh
# Packaging tools are pinned so that packages do not change with new upstream versions.
sudo gem install fpm -v 1.18.0
sudo apt-get install -y rpm
make package
