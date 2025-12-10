<?php
  $config['db_dsnw'] = 'pgsql://billionmail:PW1sHKFD5i0ArO4xVWn9a2IQ6liw6kAq@pgsql:5432/billionmail';
  $config['db_dsnr'] = '';
  $config['imap_host'] = 'dovecot:143';
  $config['smtp_host'] = 'postfix:25';
  $config['username_domain'] = '';
  $config['temp_dir'] = '/tmp/roundcube-temp';
  $config['skin'] = 'elastic';
  $config['request_path'] = '/roundcube';
  $config['plugins'] = array_filter(array_unique(array_merge($config['plugins'], ['archive', 'zipdownload'])));
  
include('/var/roundcube/config/custom.inc.php');
