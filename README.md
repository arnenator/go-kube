# go-kube
go-kube is a wrapper over the kubectl commands such that we can have a nice interface that we can call from our go code.

**Why is this repo created?**

We (@andreaswachs and @Arneproductions) experienced many times while developing tools for Kubernetes that we were missing the ability to just call an apply or delete function that had the same behavior as Kubectl. We found ourselves, trying to implement the exact same logic as kubectl apply over and over again. This is of course a hassle, since we need to figure out how to create a specific resource or how to patch existing resources and choosing the correct patch method. 

So we asked ourself... why reinvent the wheel? Kubectl has some nice commands that does this for us. But the code around kubectl can be really cumbersome and hard to understand. So for that reason we have implemented this little tool that simplifies the interface for all us developers.

## Get started
In order to get started you need to get the package to your project.
```
go get github.com/Arneproductions/go-kube
```

Now in order to use the functions you need to import the following package in your go file:
```go
import github.com/Arneproductions/go-kube/pkg/kubectl
```

This lets you call the functions like the following:
```go
opts := kubectl.ApplyManifestsOptions{
    Recursive: false
}

kubectl.ApplyManifests(ctx, pathToKubeconfig, opts, pathToManifests...)
```
