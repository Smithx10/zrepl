package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fatih/color"
	"github.com/pkg/errors"

	"github.com/zrepl/zrepl/config"
	"github.com/zrepl/zrepl/daemon/logging"
	"github.com/zrepl/zrepl/logger"
	"github.com/zrepl/zrepl/platformtest"
	"github.com/zrepl/zrepl/platformtest/tests"
)

func main() {

	var args struct {
		createArgs            platformtest.ZpoolCreateArgs
		stopAndKeepPoolOnFail bool
	}
	flag.StringVar(&args.createArgs.PoolName, "poolname", "", "")
	flag.StringVar(&args.createArgs.ImagePath, "imagepath", "", "")
	flag.Int64Var(&args.createArgs.ImageSize, "imagesize", 100*(1<<20), "")
	flag.StringVar(&args.createArgs.Mountpoint, "mountpoint", "none", "")
	flag.BoolVar(&args.stopAndKeepPoolOnFail, "failure.stop-and-keep-pool", false, "if a test case fails, stop test execution and keep pool as it was when the test failed")
	flag.Parse()

	outlets := logger.NewOutlets()
	outlet, level, err := logging.ParseOutlet(config.LoggingOutletEnum{Ret: &config.StdoutLoggingOutlet{
		LoggingOutletCommon: config.LoggingOutletCommon{
			Level:  "debug",
			Format: "human",
		},
	}})
	if err != nil {
		panic(err)
	}
	outlets.Add(outlet, level)
	logger := logger.NewLogger(outlets, 1*time.Second)

	if err := args.createArgs.Validate(); err != nil {
		logger.Error(err.Error())
		panic(err)
	}
	ctx := platformtest.WithLogger(context.Background(), logger)
	ex := platformtest.NewEx(logger)

	bold := color.New(color.Bold)
	boldRed := color.New(color.Bold, color.FgHiRed)
	boldGreen := color.New(color.Bold, color.FgHiGreen)
	for _, c := range tests.Cases {
		// ATTENTION future parallelism must pass c by value into closure!
		err := func() (err error) {
			var oErr = &err
			bold.Printf("BEGIN TEST CASE %s\n", c)
			pool, err := platformtest.CreateOrReplaceZpool(ctx, ex, args.createArgs)
			if err != nil {
				return errors.Wrap(err, "create test pool")
			}
			defer func() {
				if err := recover(); err != nil {
					*oErr = errors.Errorf("panic while running test: %v", err) // shadow
					if args.stopAndKeepPoolOnFail {
						return
					}
					// fallthrough
				}
				if err := pool.Destroy(ctx, ex); err != nil {
					fmt.Printf("error destroying test pool: %s", err)
				}
			}()
			ctx := &platformtest.Context{
				Context:     ctx,
				RootDataset: filepath.Join(pool.Name(), "rootds"),
			}
			c(ctx)
			return nil
		}()
		if err != nil {
			boldRed.Printf("TEST CASE FAILED WITH ERROR:\n")
			fmt.Printf("%+v\n", err) // print with stack trace
			if args.stopAndKeepPoolOnFail {
				bold.Printf("STOPPING TEST RUN AT FAILING TEST PER USER REQUEST\n")
				return
			}
		} else {
			boldGreen.Printf("DONE  TEST CASE %s\n", c)
		}
		fmt.Println()
	}

}
